// Modifications Copyright 2020 The klaytn Authors
// Copyright 2019 The go-ethereum Authors
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
//
// This file is derived from core/state_prefetcher.go (2019/04/02).
// Modified and improved for the klaytn development.

package blockchain

import (
	"sync/atomic"

	"github.com/klaytn/klaytn/blockchain/state"
	"github.com/klaytn/klaytn/blockchain/types"
	"github.com/klaytn/klaytn/blockchain/vm"
	"github.com/klaytn/klaytn/common"
	"github.com/klaytn/klaytn/consensus"
	"github.com/klaytn/klaytn/params"
)

// statePrefetcher is a basic Prefetcher, which blindly executes a block on top
// of an arbitrary state with the goal of prefetching potentially useful state
// data from disk before the main block processor start executing.
type statePrefetcher struct {
	config *params.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
}

// newStatePrefetcher initialises a new statePrefetcher.
func newStatePrefetcher(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine) *statePrefetcher {
	return &statePrefetcher{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

// Prefetch processes the state changes according to the Klaytn rules by running
// the transaction messages using the statedb, but any changes are discarded. The
// only goal is to pre-cache transaction signatures and state trie nodes.
func (p *statePrefetcher) Prefetch(block *types.Block, stateDB *state.StateDB, cfg vm.Config, interrupt *uint32) {
	var (
		header = block.Header()
	)
	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {
		// If block precaching was interrupted, abort
		if interrupt != nil && atomic.LoadUint32(interrupt) == 1 {
			return
		}
		// Block precaching permitted to continue, execute the transaction
		stateDB.Prepare(tx.Hash(), block.Hash(), i)
		if err := precacheTransaction(p.config, p.bc, nil, stateDB, header, tx, cfg); err != nil {
			return // Ugh, something went horribly wrong, bail out
		}
	}
}

// PrefetchTx processes the state changes according to the Klaytn rules by running
// a single transaction message using the statedb, but any changes are discarded. The
// only goal is to pre-cache transaction signatures and state trie nodes. It is used
// when fetcher works, so it fetches only a block.
func (p *statePrefetcher) PrefetchTx(block *types.Block, ti int, stateDB *state.StateDB, cfg vm.Config, interrupt *uint32) {
	var (
		header = block.Header()
		tx     = block.Transactions()[ti]
	)

	// If block precaching was interrupted, abort
	if interrupt != nil && atomic.LoadUint32(interrupt) == 1 {
		return
	}

	// Block precaching permitted to continue, execute the transaction
	stateDB.Prepare(tx.Hash(), block.Hash(), ti)
	if err := precacheTransaction(p.config, p.bc, nil, stateDB, header, tx, cfg); err != nil {
		return // Ugh, something went horribly wrong, bail out
	}
}

// precacheTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. The goal is not to execute
// the transaction successfully, rather to warm up touched data slots.
func precacheTransaction(config *params.ChainConfig, bc ChainContext, author *common.Address, statedb *state.StateDB, header *types.Header, tx *types.Transaction, cfg vm.Config) error {
	// Convert the transaction into an executable message and pre-cache its sender
	msg, err := tx.AsMessageWithAccountKeyPicker(types.MakeSigner(config, header.Number), statedb, header.Number.Uint64())
	if err != nil {
		return err
	}
	// Create the EVM and execute the transaction
	context := NewEVMContext(msg, header, bc, author)
	vm := vm.NewEVM(context, statedb, config, &cfg)

	_, _, kerr := ApplyMessage(vm, msg)
	return kerr.ErrTxInvalid
}
