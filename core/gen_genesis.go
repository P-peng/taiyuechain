// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package core

import (
	"encoding/json"
	"errors"

	"github.com/taiyuechain/taiyuechain/common"
	"github.com/taiyuechain/taiyuechain/common/hexutil"
	"github.com/taiyuechain/taiyuechain/common/math"
	"github.com/taiyuechain/taiyuechain/core/types"
	"github.com/taiyuechain/taiyuechain/params"
)

var _ = (*genesisSpecMarshaling)(nil)

// MarshalJSON marshals as JSON.
func (g Genesis) MarshalJSON() ([]byte, error) {
	type Genesis struct {
		Config       *params.ChainConfig                               `json:"config"`
		Timestamp    math.HexOrDecimal64                               `json:"timestamp"`
		ExtraData    hexutil.Bytes                                     `json:"extraData"`
		GasLimit     math.HexOrDecimal64                               `json:"gasLimit"   gencodec:"required"`
		UseGas       uint8                                             `json:"useGas" 		gencodec:"required"`
		IsCoin   uint8                                             `json:"isCoin" 		gencodec:"required"`
		KindOfCrypto uint8                                             `json:"kindOfCrypto" 		gencodec:"required"`
		PermisionWlSendTx	 uint8												 `json:"permisionWlSendTx" 		gencodec:"required"`
		PermisionWlCreateTx   uint8												 `json:"permisionWlCreateTx" 		gencodec:"required"`
		Coinbase     common.Address                                    `json:"coinbase"`
		Alloc        map[common.UnprefixedAddress]types.GenesisAccount `json:"alloc"`
		Committee    []*types.CommitteeMember                          `json:"committee"      gencodec:"required"`
		CertList     []hexutil.Bytes                                   `json:"CertList"      gencodec:"required"`
		Number       math.HexOrDecimal64                               `json:"number"`
		GasUsed      math.HexOrDecimal64                               `json:"gasUsed"`
		ParentHash   common.Hash                                       `json:"parentHash"`
	}
	var enc Genesis
	enc.Config = g.Config
	enc.Timestamp = math.HexOrDecimal64(g.Timestamp)
	enc.ExtraData = g.ExtraData
	enc.GasLimit = math.HexOrDecimal64(g.GasLimit)
	enc.UseGas = g.UseGas
	enc.IsCoin = g.IsCoin
	enc.PermisionWlSendTx = g.PermisionWlSendTx
	enc.PermisionWlCreateTx = g.PermisionWlCreateTx
	enc.KindOfCrypto = g.KindOfCrypto
	enc.Coinbase = g.Coinbase
	if g.Alloc != nil {
		enc.Alloc = make(map[common.UnprefixedAddress]types.GenesisAccount, len(g.Alloc))
		for k, v := range g.Alloc {
			enc.Alloc[common.UnprefixedAddress(k)] = v
		}
	}
	enc.Committee = g.Committee
	if g.CertList != nil {
		enc.CertList = make([]hexutil.Bytes, len(g.CertList))
		for k, v := range g.CertList {
			enc.CertList[k] = v
		}
	}
	enc.Number = math.HexOrDecimal64(g.Number)
	enc.GasUsed = math.HexOrDecimal64(g.GasUsed)
	enc.ParentHash = g.ParentHash
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (g *Genesis) UnmarshalJSON(input []byte) error {
	type Genesis struct {
		Config       *params.ChainConfig                               `json:"config"`
		Timestamp    *math.HexOrDecimal64                              `json:"timestamp"`
		ExtraData    *hexutil.Bytes                                    `json:"extraData"`
		GasLimit     *math.HexOrDecimal64                              `json:"gasLimit"   gencodec:"required"`
		UseGas       *uint8                                            `json:"useGas" 		gencodec:"required"`
		IsCoin   *uint8                                            `json:"isCoin" 		gencodec:"required"`
		KindOfCrypto *uint8                                            `json:"kindOfCrypto" 		gencodec:"required"`
		PermisionWlSendTx	 *uint8												 `json:"permisionWlSendTx" 		gencodec:"required"`
		PermisionWlCreateTx   *uint8												 `json:"permisionWlCreateTx" 		gencodec:"required"`
		Coinbase     *common.Address                                   `json:"coinbase"`
		Alloc        map[common.UnprefixedAddress]types.GenesisAccount `json:"alloc"`
		Committee    []*types.CommitteeMember                          `json:"committee"      gencodec:"required"`
		CertList     []hexutil.Bytes                                   `json:"CertList"      gencodec:"required"`
		Number       *math.HexOrDecimal64                              `json:"number"`
		GasUsed      *math.HexOrDecimal64                              `json:"gasUsed"`
		ParentHash   *common.Hash                                      `json:"parentHash"`
	}
	var dec Genesis
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Config != nil {
		g.Config = dec.Config
	}
	if dec.Timestamp != nil {
		g.Timestamp = uint64(*dec.Timestamp)
	}
	if dec.ExtraData != nil {
		g.ExtraData = *dec.ExtraData
	}
	if dec.GasLimit == nil {
		return errors.New("missing required field 'gasLimit' for Genesis")
	}
	g.GasLimit = uint64(*dec.GasLimit)
	if dec.UseGas != nil {
		g.UseGas = *dec.UseGas
	}
	if dec.IsCoin != nil {
		g.IsCoin = *dec.IsCoin
	}
	if dec.PermisionWlSendTx != nil {
		g.PermisionWlSendTx = *dec.PermisionWlSendTx
	}
	if dec.PermisionWlCreateTx != nil {
		g.PermisionWlCreateTx = *dec.PermisionWlCreateTx
	}
	if dec.KindOfCrypto != nil {
		g.KindOfCrypto = *dec.KindOfCrypto
	}
	if dec.Coinbase != nil {
		g.Coinbase = *dec.Coinbase
	}
	if dec.Alloc != nil {
		g.Alloc = make(types.GenesisAlloc, len(dec.Alloc))
		for k, v := range dec.Alloc {
			g.Alloc[common.Address(k)] = v
		}
	}
	if dec.Committee == nil {
		return errors.New("missing required field 'committee' for Genesis")
	}
	g.Committee = dec.Committee
	if dec.CertList == nil {
		return errors.New("missing required field 'CertList' for Genesis")
	}
	g.CertList = make([][]byte, len(dec.CertList))
	for k, v := range dec.CertList {
		g.CertList[k] = v
	}
	if dec.Number != nil {
		g.Number = uint64(*dec.Number)
	}
	if dec.GasUsed != nil {
		g.GasUsed = uint64(*dec.GasUsed)
	}
	if dec.ParentHash != nil {
		g.ParentHash = *dec.ParentHash
	}
	return nil
}
