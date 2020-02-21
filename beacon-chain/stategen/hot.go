package stategen

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/prysmaticlabs/go-ssz"
	"github.com/prysmaticlabs/prysm/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/beacon-chain/db/filters"
	"github.com/prysmaticlabs/prysm/beacon-chain/state"
	pb "github.com/prysmaticlabs/prysm/proto/beacon/p2p/v1"
	"github.com/prysmaticlabs/prysm/shared/bytesutil"
	"github.com/prysmaticlabs/prysm/shared/params"
	"github.com/sirupsen/logrus"
)

// HotStateExists returns true if the corresponding state of the input block either
// exists in the DB or it can be generated by state gen.
func (s *State) HotStateExists(ctx context.Context, blockRoot [32]byte) bool {
	return s.beaconDB.HasHotStateSummary(ctx, blockRoot)
}

// This saves a post finalized beacon state in the hot section of the DB. On the epoch boundary,
// it saves a full state. On an intermediate slot, it saves a back pointer to the
// nearest epoch boundary state.
func (s *State) saveHotState(ctx context.Context, blockRoot [32]byte, state *state.BeaconState) error {
	// On an epoch boundary, saves the whole state.
	if helpers.IsEpochStart(state.Slot()) {
		if err := s.beaconDB.SaveState(ctx, state, blockRoot); err != nil {
			return err
		}
		log.WithFields(logrus.Fields{
			"slot":      state.Slot(),
			"blockRoot": hex.EncodeToString(bytesutil.Trunc(blockRoot[:]))}).Info("Saved full state on epoch boundary")
		hotStateSaved.Inc()
	}

	// On an intermediate slot, save the state summary.
	epochRoot, err := s.loadEpochBoundaryRoot(ctx, blockRoot, state)
	if err != nil {
		return err
	}
	if err := s.beaconDB.SaveHotStateSummary(ctx, &pb.HotStateSummary{
		Slot:         state.Slot(),
		LatestRoot:   blockRoot[:],
		BoundaryRoot: epochRoot[:],
	}); err != nil {
		return err
	}
	hotSummarySaved.Inc()

	// Store the state in the cache.

	return nil
}

// This loads a post finalized beacon state from the hot section of the DB. If necessary it will
// replay blocks from the nearest epoch boundary.
func (s *State) loadHotStateByRoot(ctx context.Context, blockRoot [32]byte) (*state.BeaconState, error) {
	// Load the cache

	summary, err := s.beaconDB.HotStateSummary(ctx, blockRoot)
	if err != nil {
		return nil, err
	}
	if summary == nil {
		return nil, errors.New("nil hot state summary")
	}
	targetSlot := summary.Slot

	boundaryState, err := s.beaconDB.State(ctx, bytesutil.ToBytes32(summary.BoundaryRoot))
	if err != nil {
		return nil, err
	}
	if boundaryState == nil {
		return nil, errors.New("boundary state can't be nil")
	}

	// Don't need to replay the blocks if we're already on an epoch boundary.
	var hotState *state.BeaconState
	if helpers.IsEpochStart(targetSlot) {
		hotState = boundaryState
	} else {
		blks, err := s.LoadBlocks(ctx, boundaryState.Slot()+1, targetSlot, bytesutil.ToBytes32(summary.LatestRoot))
		if err != nil {
			return nil, err
		}
		hotState, err = s.ReplayBlocks(ctx, boundaryState, blks, targetSlot)
		if err != nil {
			return nil, err
		}
	}

	// Save the cache

	return hotState, nil
}

// This loads a hot state by slot only where the slot lies between the epoch boundary points.
// This is a slower implementation given slot is the only argument. It require fetching
// all the blocks between the epoch boundary points.
func (s *State) loadHotIntermediateStateWithSlot(ctx context.Context, slot uint64) (*state.BeaconState, error) {
	// Gather epoch boundary information, this is where node starts to replay the blocks.
	epochBoundarySlot := helpers.StartSlot(helpers.SlotToEpoch(slot))
	epochBoundaryRoot, ok := s.epochBoundaryRoot(epochBoundarySlot)
	if !ok {
		return nil, errors.New("epoch boundary root is not cached")
	}
	epochBoundaryState, err := s.beaconDB.State(ctx, epochBoundaryRoot)
	if err != nil {
		return nil, err
	}

	// Gather the last physical block root and the slot number.
	lastValidRoot, lastValidSlot, err := s.getLastValidBlock(ctx, slot)
	if err != nil {
		return nil, err
	}

	// Load and replay blocks to get the intermediate state.
	replayBlks, err := s.LoadBlocks(ctx, epochBoundaryState.Slot()+1, lastValidSlot, lastValidRoot)
	if err != nil {
		return nil, err
	}
	return s.ReplayBlocks(ctx, epochBoundaryState, replayBlks, slot)
}

// This loads the epoch boundary root of a given state based on the state slot.
// If the epoch boundary does not have a valid block, it goes back to find the last
// slot which has a valid block.
func (s *State) loadEpochBoundaryRoot(ctx context.Context, blockRoot [32]byte, state *state.BeaconState) ([32]byte, error) {
	epochBoundarySlot := helpers.CurrentEpoch(state) * params.BeaconConfig().SlotsPerEpoch

	// Node first checks if epoch boundary root already exists in cache.
	r, ok := s.epochBoundarySlotToRoot[epochBoundarySlot]
	if ok {
		return r, nil
	}

	// At epoch boundary, the root is just itself.
	if state.Slot() == epochBoundarySlot {
		return blockRoot, nil
	}

	// Node uses genesis getters if the epoch boundary slot is on genesis slot.
	if epochBoundarySlot == 0 {
		b, err := s.beaconDB.GenesisBlock(ctx)
		if err != nil {
			return [32]byte{}, err
		}

		r, err = ssz.HashTreeRoot(b.Block)
		if err != nil {
			return [32]byte{}, err
		}

		s.setEpochBoundaryRoot(epochBoundarySlot, r)

		return r, nil
	}

	// Now to find the epoch boundary root via DB.
	filter := filters.NewFilter().SetStartSlot(epochBoundarySlot).SetEndSlot(epochBoundarySlot)
	rs, err := s.beaconDB.BlockRoots(ctx, filter)
	if err != nil {
		return [32]byte{}, err
	}
	// If the epoch boundary is a skip slot, traverse back to find the last valid state.
	if len(rs) == 0 {
		r, err = s.handleLastValidState(ctx, epochBoundarySlot)
		if err != nil {
			return [32]byte{}, err
		}
	} else if len(rs) == 1 {
		r = rs[0]
	} else {
		// This should not happen, there shouldn't be more than 1 epoch boundary root,
		// but adding this check to be save.
		return [32]byte{}, errors.New("incorrect length for epoch boundary root")
	}

	// Set the epoch boundary root cache.
	s.setEpochBoundaryRoot(epochBoundarySlot, r)

	return r, nil
}

// This finds the last valid state from searching backwards starting at input slot
// and returns the root of the block which is used to process the state.
func (s *State) handleLastValidState(ctx context.Context, slot uint64) ([32]byte, error) {
	filter := filters.NewFilter().SetStartSlot(s.splitInfo.slot).SetEndSlot(slot)
	// We know the epoch boundary root will be the last index using the filter.
	rs, err := s.beaconDB.BlockRoots(ctx, filter)
	if err != nil {
		return [32]byte{}, err
	}
	lastRoot := rs[len(rs)-1]

	// Node replays to get the last valid state which has a block.
	// Then saves the state in the DB.
	r, _ := s.epochBoundaryRoot(slot - params.BeaconConfig().SlotsPerEpoch)
	startState, err := s.beaconDB.State(ctx, r)
	if err != nil {
		return [32]byte{}, err
	}
	blks, err := s.LoadBlocks(ctx, startState.Slot()+1, slot, lastRoot)
	if err != nil {
		return [32]byte{}, err
	}
	startState, err = s.ReplayBlocks(ctx, startState, blks, slot)
	if err != nil {
		return [32]byte{}, err
	}
	if err := s.beaconDB.SaveState(ctx, startState, lastRoot); err != nil {
		return [32]byte{}, err
	}

	return lastRoot, nil
}

// This finds the last valid block from searching backwards starting at input slot
// and returns the root of the block.
func (s *State) getLastValidBlock(ctx context.Context, targetSlot uint64) ([32]byte, uint64, error) {
	filter := filters.NewFilter().SetStartSlot(s.splitInfo.slot).SetEndSlot(targetSlot)
	// We know the epoch boundary root will be the last index using the filter.
	rs, err := s.beaconDB.BlockRoots(ctx, filter)
	if err != nil {
		return [32]byte{}, 0, err
	}
	lastRoot := rs[len(rs)-1]

	b, err := s.beaconDB.Block(ctx, lastRoot)
	if err != nil {
		return [32]byte{}, 0, err
	}

	return lastRoot, b.Block.Slot, nil
}
