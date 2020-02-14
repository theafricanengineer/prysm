package stategen

import (
	"context"
	"reflect"
	"testing"

	ethpb "github.com/prysmaticlabs/ethereumapis/eth/v1alpha1"
	"github.com/prysmaticlabs/go-ssz"
	"github.com/prysmaticlabs/prysm/beacon-chain/db"
	testDB "github.com/prysmaticlabs/prysm/beacon-chain/db/testing"
	stateTrie "github.com/prysmaticlabs/prysm/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/shared/bytesutil"
)

func TestLoadBlocks_FirstBranch(t *testing.T) {
	db := testDB.SetupDB(t)
	defer testDB.TeardownDB(t, db)
	ctx := context.Background()
	s := &State{
		beaconDB: db,
	}

	roots, savedBlocks, err := tree1(db, []byte{'A'})
	if err != nil {
		t.Fatal(err)
	}

	filteredBlocks, err := s.loadBlocks(ctx, 0, 8, roots[len(roots)-1])
	if err != nil {
		t.Fatal(err)
	}

	wanted := []*ethpb.SignedBeaconBlock{
		{Block: savedBlocks[8]},
		{Block: savedBlocks[6]},
		{Block: savedBlocks[4]},
		{Block: savedBlocks[2]},
		{Block: savedBlocks[1]},
		{Block: savedBlocks[0]},
	}
	if !reflect.DeepEqual(filteredBlocks, wanted) {
		t.Error("Did not get wanted blocks")
	}
}

func TestLoadBlocks_SecondBranch(t *testing.T) {
	db := testDB.SetupDB(t)
	defer testDB.TeardownDB(t, db)
	ctx := context.Background()
	s := &State{
		beaconDB: db,
	}

	roots, savedBlocks, err := tree1(db, []byte{'A'})
	if err != nil {
		t.Fatal(err)
	}

	filteredBlocks, err := s.loadBlocks(ctx, 0, 5, roots[5])
	if err != nil {
		t.Fatal(err)
	}

	wanted := []*ethpb.SignedBeaconBlock{
		{Block: savedBlocks[5]},
		{Block: savedBlocks[3]},
		{Block: savedBlocks[1]},
		{Block: savedBlocks[0]},
	}
	if !reflect.DeepEqual(filteredBlocks, wanted) {
		t.Error("Did not get wanted blocks")
	}
}

func TestLoadBlocks_ThirdBranch(t *testing.T) {
	db := testDB.SetupDB(t)
	defer testDB.TeardownDB(t, db)
	ctx := context.Background()
	s := &State{
		beaconDB: db,
	}

	roots, savedBlocks, err := tree1(db, []byte{'A'})
	if err != nil {
		t.Fatal(err)
	}

	filteredBlocks, err := s.loadBlocks(ctx, 0, 7, roots[7])
	if err != nil {
		t.Fatal(err)
	}

	wanted := []*ethpb.SignedBeaconBlock{
		{Block: savedBlocks[7]},
		{Block: savedBlocks[6]},
		{Block: savedBlocks[4]},
		{Block: savedBlocks[2]},
		{Block: savedBlocks[1]},
		{Block: savedBlocks[0]},
	}
	if !reflect.DeepEqual(filteredBlocks, wanted) {
		t.Error("Did not get wanted blocks")
	}
}

func TestLoadBlocks_SameSlots(t *testing.T) {
	db := testDB.SetupDB(t)
	defer testDB.TeardownDB(t, db)
	ctx := context.Background()
	s := &State{
		beaconDB: db,
	}

	roots, savedBlocks, err := tree2(db, []byte{'A'})
	if err != nil {
		t.Fatal(err)
	}

	filteredBlocks, err := s.loadBlocks(ctx, 0, 3, roots[6])
	if err != nil {
		t.Fatal(err)
	}

	wanted := []*ethpb.SignedBeaconBlock{
		{Block: savedBlocks[6]},
		{Block: savedBlocks[5]},
		{Block: savedBlocks[1]},
		{Block: savedBlocks[0]},
	}
	if !reflect.DeepEqual(filteredBlocks, wanted) {
		t.Error("Did not get wanted blocks")
	}
}

func TestLoadBlocks_BadStart(t *testing.T) {
	db := testDB.SetupDB(t)
	defer testDB.TeardownDB(t, db)
	ctx := context.Background()
	s := &State{
		beaconDB: db,
	}

	roots, _, err := tree1(db, []byte{'A'})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.loadBlocks(ctx, 0, 5, roots[8])
	if err.Error() != "end block roots don't match" {
		t.Error("Did not get wanted error")
	}
}

// tree1 constructs the following tree:
// B0 - B1 - - B3 -- B5
//        \- B2 -- B4 -- B6 ----- B8
//                         \- B7
func tree1(db db.Database, genesisRoot []byte) ([][32]byte, []*ethpb.BeaconBlock, error) {
	b0 := &ethpb.BeaconBlock{Slot: 0, ParentRoot: genesisRoot}
	r0, _ := ssz.HashTreeRoot(b0)
	b1 := &ethpb.BeaconBlock{Slot: 1, ParentRoot: r0[:]}
	r1, _ := ssz.HashTreeRoot(b1)
	b2 := &ethpb.BeaconBlock{Slot: 2, ParentRoot: r1[:]}
	r2, _ := ssz.HashTreeRoot(b2)
	b3 := &ethpb.BeaconBlock{Slot: 3, ParentRoot: r1[:]}
	r3, _ := ssz.HashTreeRoot(b3)
	b4 := &ethpb.BeaconBlock{Slot: 4, ParentRoot: r2[:]}
	r4, _ := ssz.HashTreeRoot(b4)
	b5 := &ethpb.BeaconBlock{Slot: 5, ParentRoot: r3[:]}
	r5, _ := ssz.HashTreeRoot(b5)
	b6 := &ethpb.BeaconBlock{Slot: 6, ParentRoot: r4[:]}
	r6, _ := ssz.HashTreeRoot(b6)
	b7 := &ethpb.BeaconBlock{Slot: 7, ParentRoot: r6[:]}
	r7, _ := ssz.HashTreeRoot(b7)
	b8 := &ethpb.BeaconBlock{Slot: 8, ParentRoot: r6[:]}
	r8, _ := ssz.HashTreeRoot(b8)
	for _, b := range []*ethpb.BeaconBlock{b0, b1, b2, b3, b4, b5, b6, b7, b8} {
		if err := db.SaveBlock(context.Background(), &ethpb.SignedBeaconBlock{Block: b}); err != nil {
			return nil, nil, err
		}
		if err := db.SaveState(context.Background(), &stateTrie.BeaconState{}, bytesutil.ToBytes32(b.ParentRoot)); err != nil {
			return nil, nil, err
		}
	}
	return [][32]byte{r0, r1, r2, r3, r4, r5, r6, r7, r8}, []*ethpb.BeaconBlock{b0, b1, b2, b3, b4, b5, b6, b7, b8}, nil
}

// tree2 constructs the following tree:
// B0 - B1
//        \- B2
//        \- B2
//        \- B2
//        \- B2 -- B3
func tree2(db db.Database, genesisRoot []byte) ([][32]byte, []*ethpb.BeaconBlock, error) {
	b0 := &ethpb.BeaconBlock{Slot: 0, ParentRoot: genesisRoot}
	r0, _ := ssz.HashTreeRoot(b0)
	b1 := &ethpb.BeaconBlock{Slot: 1, ParentRoot: r0[:]}
	r1, _ := ssz.HashTreeRoot(b1)
	b21 := &ethpb.BeaconBlock{Slot: 2, ParentRoot: r1[:], StateRoot: []byte{'A'}}
	r21, _ := ssz.HashTreeRoot(b21)
	b22 := &ethpb.BeaconBlock{Slot: 2, ParentRoot: r1[:], StateRoot: []byte{'B'}}
	r22, _ := ssz.HashTreeRoot(b22)
	b23 := &ethpb.BeaconBlock{Slot: 2, ParentRoot: r1[:], StateRoot: []byte{'C'}}
	r23, _ := ssz.HashTreeRoot(b23)
	b24 := &ethpb.BeaconBlock{Slot: 2, ParentRoot: r1[:], StateRoot: []byte{'D'}}
	r24, _ := ssz.HashTreeRoot(b24)
	b3 := &ethpb.BeaconBlock{Slot: 3, ParentRoot: r24[:]}
	r3, _ := ssz.HashTreeRoot(b3)

	for _, b := range []*ethpb.BeaconBlock{b0, b1, b21, b22, b23, b24, b3} {
		if err := db.SaveBlock(context.Background(), &ethpb.SignedBeaconBlock{Block: b}); err != nil {
			return nil, nil, err
		}
		if err := db.SaveState(context.Background(), &stateTrie.BeaconState{}, bytesutil.ToBytes32(b.ParentRoot)); err != nil {
			return nil, nil, err
		}
	}
	return [][32]byte{r0, r1, r21, r22, r23, r24, r3}, []*ethpb.BeaconBlock{b0, b1, b21, b22, b23, b24, b3}, nil
}
