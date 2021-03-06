// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package minerva implements the taiyuechain hybrid consensus engine.
package minerva

import (
	"bytes"
	//"crypto/ecdsa"

	//"crypto/ecdsa"
	"github.com/taiyuechain/taiyuechain/crypto"

	"errors"

	"github.com/taiyuechain/taiyuechain/common"

	//"github.com/taiyuechain/taiyuechain/crypto"
	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/taiyuechain/taiyuechain/consensus"
	"github.com/taiyuechain/taiyuechain/core/types"
	"github.com/taiyuechain/taiyuechain/log"

	//"github.com/taiyuechain/taiyuechain/metrics"
	"github.com/taiyuechain/taiyuechain/rpc"
	//"golang.org/x/crypto/sha3"
	"crypto/ecdsa"
	"math/big"

	//"math/rand"
	"sync"
	"time"

	"github.com/taiyuechain/taiyuechain/cim"
)

// ErrInvalidDumpMagic errorinfo
var ErrInvalidDumpMagic = errors.New("invalid dump magic")

var (
	// maxUint218 is a big integer representing 2^218-1
	maxUint128 = new(big.Int).Exp(big.NewInt(2), big.NewInt(128), big.NewInt(0))

	// sharedMinerva is a full instance that can be shared between multiple users.
	sharedMinerva = New(Config{ModeNormal},nil)

	//SnailBlockRewardsBase Snail block rewards base value is 115.555555555555 * 10^12
	SnailBlockRewardsBase = 115555555555555

	// Big1e6 up to wei  SnailBlockRewardsBase * this is wei
	Big1e6 = big.NewInt(1e6)

	// SnailBlockRewardsChangeInterval Snail block rewards change interval 4500 blocks
	SnailBlockRewardsChangeInterval = 4500

	// SnailBlockRewardsChangePercentage snall block rewards change interval decrease %2
	SnailBlockRewardsChangePercentage = 2

	//BaseBig ...
	BaseBig = big.NewInt(1e18)

	//NetworkFragmentsNuber The number of main network fragments is currently fixed at 1
	NetworkFragmentsNuber = 1

	//MiningConstant Mining constant is 20
	MiningConstant = 20

	//SqrtMin pbft and miner allocation constant
	//Generating formula :TestOutSqrt
	SqrtMin = 25

	//SqrtMax ...
	SqrtMax = 6400
)

// ConstSqrt ...
type ConstSqrt struct {
	Num  int     `json:"num"`
	Sqrt float64 `json:"sqrt"`
}

// lru tracks caches or datasets by their last use time, keeping at most N of them.
type lru struct {
	what string
	new  func(epoch uint64) interface{}
	mu   sync.Mutex
	// Items are kept in a LRU cache, but there is a special case:
	// We always keep an item for (highest seen epoch) + 1 as the 'future item'.
	cache      *simplelru.LRU
	future     uint64
	futureItem interface{}
}

// newlru create a new least-recently-used cache for either the verification caches
// or the mining datasets.
func newlru(what string, maxItems int, new func(epoch uint64) interface{}) *lru {
	if maxItems <= 1 {
		maxItems = 5
	}
	cache, _ := simplelru.NewLRU(maxItems, func(key, value interface{}) {
		log.Trace("Evicted minerva "+what, "epoch", key)
	})
	return &lru{what: what, new: new, cache: cache}
}

// get retrieves or creates an item for the given epoch. The first return value is always
// non-nil. The second return value is non-nil if lru thinks that an item will be useful in
// the near future.
func (lru *lru) get(epoch uint64) (item, future interface{}) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	log.Debug("get lru for dataset", "epoch", epoch)
	// Get or create the item for the requested epoch.
	item, ok := lru.cache.Get(epoch)
	if !ok {
		if lru.future > 0 && lru.future == epoch {
			item = lru.futureItem
		} else {
			log.Trace("Requiring new minerva "+lru.what, "epoch", epoch)
			item = lru.new(epoch)
		}
		lru.cache.Add(epoch, item)
	}

	// start to create a futrue dataset
	if epoch < maxEpoch-1 && lru.future < epoch+1 {
		log.Debug("creat a new futrue dataset", "epoch is ", epoch+1)
		future = lru.new(epoch + 1)
		lru.future = epoch + 1
		lru.futureItem = future
	}

	//return item, lru.futureItem
	if (epoch + 1) != lru.future {
		return item, nil
	}
	return item, lru.futureItem
}

// dataset wraps an truehash dataset with some metadata to allow easier concurrent use.
type Dataset struct {
	epoch uint64 // Epoch for which this cache is relevant
	//dump    *os.File  // File descriptor of the memory mapped cache
	//mmap    mmap.MMap // Memory map itself to unmap before releasing
	dataset     []uint64  // The actual cache data content
	once        sync.Once // Ensures the cache is generated only once
	dateInit    int
	consistent  common.Hash // Consistency of generated data
	datasetHash string      // dataset hash
}

// newDataset creates a new truehash mining dataset
func NewDataset(epoch uint64) interface{} {

	ds := &Dataset{
		epoch:    epoch,
		dateInit: 0,
		dataset:  make([]uint64, TBLSIZE*DATALENGTH*PMTSIZE*32),
	}
	log.Info("create a new dateset", "epoch", epoch)

	return ds
}

// Mode defines the type and amount of PoW verification an minerva engine makes.
type Mode uint

// constant
const (
	ModeNormal Mode = iota
	ModeShared
	ModeTest
	ModeFake
	ModeFullFake
)

func ToMinervaMode(val int) Mode {
	m := Mode(val)
	switch m {
	case ModeFake:
		return ModeFake
	case ModeShared:
		return ModeShared
	case ModeTest:
		return ModeTest
	case ModeFullFake:
		return ModeFullFake
	default:
		return ModeNormal
	}
}

// Config are the configuration parameters of the minerva.
type Config struct {
	PowMode Mode
}

// Minerva consensus
type Minerva struct {
	config Config

	//caches   *lru // In memory caches to avoid regenerating too often
	//datasets *lru // In memory datasets to avoid regenerating too often

	// Mining related fields
	/*rand     *rand.Rand    // Properly seeded random source for nonces
	threads  int           // Number of threads to mine on if mining
	update   chan struct{} // Notification channel to update mining parameters
	hashrate metrics.Meter // Meter tracking the average hashrate*/

	// The fields below are hooks for testing
	shared    *Minerva      // Shared PoW verifier to avoid cache regeneration
	fakeFail  uint64        // Block number which fails PoW check even in fake mode
	fakeDelay time.Duration // Time delay to sleep for before returning from verify

	lock sync.Mutex // Ensures thread safety for the in-memory caches and mining fields

	election consensus.CommitteeElection
	certList *cim.CimList
}

//var MinervaLocal *Minerva

// New creates a full sized minerva hybrid consensus scheme.
func New(config Config,cimList *cim.CimList) *Minerva {
	minerva := &Minerva{
		config: config,
		certList: cimList,
	}
	return minerva
}

func (d *Dataset) Hash() common.Hash {
	return rlpHash(d.dataset)
}

//SetElection Append interface CommitteeElection after instantiation
func (m *Minerva) SetElection(e consensus.CommitteeElection) {
	m.election = e
}

// GetElection return election
func (m *Minerva) GetElection() consensus.CommitteeElection {
	return m.election

}

// NewTester creates a small sized minerva scheme useful only for testing
// purposes.
func NewTester(cimList *cim.CimList) *Minerva {
	return New(Config{PowMode: ModeTest},cimList)
}

// NewFaker creates a minerva consensus engine with a fake PoW scheme that accepts
// all blocks' seal as valid, though they still have to conform to the Ethereum
// consensus rules.
func NewFaker(cimList *cim.CimList) *Minerva {
	return &Minerva{
		config: Config{
			PowMode: ModeFake,
		},
		certList: cimList,
		election: newFakeElection(),
	}
}

// NewFakeFailer creates a minerva consensus engine with a fake PoW scheme that
// accepts all blocks as valid apart from the single one specified, though they
// still have to conform to the Ethereum consensus rules.
func NewFakeFailer(fail uint64) *Minerva {
	return &Minerva{
		config: Config{
			PowMode: ModeFake,
		},
		fakeFail: fail,
		election: newFakeElection(),
	}
}

// NewFakeDelayer creates a minerva consensus engine with a fake PoW scheme that
// accepts all blocks as valid, but delays verifications by some time, though
// they still have to conform to the Ethereum consensus rules.
func NewFakeDelayer(delay time.Duration) *Minerva {
	return &Minerva{
		config: Config{
			PowMode: ModeFake,
		},
		fakeDelay: delay,
		election:  newFakeElection(),
	}
}

// NewFullFaker creates an minerva consensus engine with a full fake scheme that
// accepts all blocks as valid, without checking any consensus rules whatsoever.
func NewFullFaker() *Minerva {
	return &Minerva{
		config: Config{
			PowMode: ModeFullFake,
		},
	}
}

// NewShared creates a full sized minerva shared between all requesters running
// in the same process.
func NewShared(cimList *cim.CimList) *Minerva {
	return &Minerva{shared: sharedMinerva,certList: cimList}
}

// Threads returns the number of mining threads currently enabled. This doesn't
// necessarily mean that mining is running!
/*func (m *Minerva) Threads() int {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.threads
}*/

// SetThreads updates the number of mining threads currently enabled. Calling
// this method does not start mining, only sets the thread count. If zero is
// specified, the miner will use all cores of the machine. Setting a thread
// count below zero is allowed and will cause the miner to idle, without any
// work being done.
/*func (m *Minerva) SetThreads(threads int) {
	m.lock.Lock()
	defer m.lock.Unlock()

	// If we're running a shared PoW, set the thread count on that instead
	if m.shared != nil {
		m.shared.SetThreads(threads)
		return
	}
	// Update the threads and ping any running seal to pull in any changes
	m.threads = threads
	select {
	case m.update <- struct{}{}:
	default:
	}
}*/

// Hashrate implements PoW, returning the measured rate of the search invocations
// per second over the last minute.
/*func (m *Minerva) Hashrate() float64 {
	log.Debug("minerva  hashrate", "hash", m.hashrate.Rate1())
	return m.hashrate.Rate1()
}*/

// APIs implements consensus.Engine, returning the user facing RPC APIs. Currently
// that is empty.
func (m *Minerva) APIs(chain consensus.ChainReader) []rpc.API {
	return nil
}

// SeedHash is the seed to use for generating a verification cache and the mining
// dataset.
func SeedHash(block uint64) []byte {
	return seedHash(block)
}

type fakeElection struct {
	//caoliang modify
	privates []*ecdsa.PrivateKey

	members []*types.CommitteeMember
}

func newFakeElection() *fakeElection {
	var members []*types.CommitteeMember
	pk1, err := crypto.GenerateKey()
	if err != nil {
		log.Error("initMembers", "error", err)
	}
	pk2, err := crypto.GenerateKey()
	pk3, err := crypto.GenerateKey()
	pk4, err := crypto.GenerateKey()
	pk5, err := crypto.GenerateKey()
	pk6, err := crypto.GenerateKey()
	pk7, err := crypto.GenerateKey()

	if err != nil {
		log.Error("initMembers", "error", err)
	}

	priKeys := []*ecdsa.PrivateKey{pk1, pk2, pk3, pk4, pk5, pk6, pk7}
	for _, priKey := range priKeys {

		coinbase := crypto.PubkeyToAddress(priKey.PublicKey)

		m := &types.CommitteeMember{Coinbase: coinbase, CommitteeBase: crypto.PubkeyToAddress(priKey.PublicKey), Publickey: crypto.FromECDSAPub(&priKey.PublicKey), Flag: types.StateUsedFlag, MType: types.TypeFixed}
		members = append(members, m)

	}
	return &fakeElection{privates: priKeys, members: members}
}

func (e *fakeElection) GetCommittee(fastNumber *big.Int) []*types.CommitteeMember {
	return e.members
}

func (e *fakeElection) VerifySigns(signs []*types.PbftSign) ([]*types.CommitteeMember, []error) {
	var (
		members = make([]*types.CommitteeMember, len(signs))
		errs    = make([]error, len(signs))
	)
	for i, sign := range signs {
		pubkey, _ := crypto.SigToPub(sign.HashWithNoSign().Bytes(), sign.Sign)
		pubkeyByte := crypto.FromECDSAPub(pubkey)
		for _, m := range e.members {
			if bytes.Equal(pubkeyByte, m.Publickey) {
				members[i] = m
			}
		}
	}

	return members, errs
}

// VerifySwitchInfo verify committee members and it's state
func (e *fakeElection) VerifySwitchInfo(fastnumber *big.Int, info []*types.CommitteeMember) error {
	return nil
}

func (e *fakeElection) GenerateFakeSigns(fb *types.Block) ([]*types.PbftSign, error) {
	var signs []*types.PbftSign
	var err error

	for _, privateKey := range e.privates {
		voteSign := &types.PbftSign{
			Result:     types.VoteAgree,
			FastHeight: fb.Header().Number,
			FastHash:   fb.Hash(),
		}
		signHash := voteSign.HashWithNoSign().Bytes()
		voteSign.Sign, err = crypto.Sign(signHash, privateKey)

		if err != nil {
			log.Error("fb fake GenerateSign error ", "err", err)
		}
		signs = append(signs, voteSign)
	}
	return signs, err
}

// for hash
func rlpHash(x interface{}) (h common.Hash) {
	return crypto.RlpHash(x)
}
