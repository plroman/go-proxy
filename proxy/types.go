package proxy

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
)

// Note on optional Signer field:
// * when receiving from Flashbots or other builders this field should be set
// * otherwise its set from the request signature by orderflow proxy
//   in this case it can be empty! @should we prohibit that

// eth_SendBundle

type EthSendBundleArgs struct {
	Txs               []hexutil.Bytes `json:"txs"`         // empty txs for cancellations are not supported
	BlockNumber       rpc.BlockNumber `json:"blockNumber"` // 0 block number is not supported
	MinTimestamp      *uint64         `json:"minTimestamp,omitempty"`
	MaxTimestamp      *uint64         `json:"maxTimestamp,omitempty"`
	RevertingTxHashes []common.Hash   `json:"revertingTxHashes,omitempty"`
	ReplacementUUID   *string         `json:"replacementUuid,omitempty"`

	// fields available only when receiving from the Flashbots or other builders, not users
	ReplacementNonce *uint64         `json:"replacementNonce,omitempty"`
	SigningAddress   *common.Address `json:"signingAddress,omitempty"`

	DroppingTxHashes []common.Hash   `json:"droppingTxHashes,omitempty"` // not supported (from beaverbuild)
	UUID             *string         `json:"uuid,omitempty"`             // not supported (from beaverbuild)
	RefundPercent    *uint64         `json:"refundPercent,omitempty"`    // not supported (from beaverbuild)
	RefundRecipient  *common.Address `json:"refundRecipient,omitempty"`  // not supported (from beaverbuild)
	RefundTxHashes   []string        `json:"refundTxHashes,omitempty"`   // not supported (from titanbuilder)
}

// mev_sendBundle

const (
	RevertModeAllow = "allow"
	RevertModeDrop  = "drop"
	RevertModeFail  = "fail"
)

type MevBundleInclusion struct {
	BlockNumber hexutil.Uint64 `json:"block"`
	MaxBlock    hexutil.Uint64 `json:"maxBlock"`
}

type MevBundleBody struct {
	Hash       *common.Hash       `json:"hash,omitempty"`
	Tx         *hexutil.Bytes     `json:"tx,omitempty"`
	Bundle     *MevSendBundleArgs `json:"bundle,omitempty"`
	CanRevert  bool               `json:"canRevert,omitempty"`
	RevertMode string             `json:"revertMode,omitempty"`
}

type MevBundleValidity struct {
	Refund       []RefundConstraint `json:"refund,omitempty"`
	RefundConfig []RefundConfig     `json:"refundConfig,omitempty"`
}

type RefundConstraint struct {
	BodyIdx int `json:"bodyIdx"`
	Percent int `json:"percent"`
}

type RefundConfig struct {
	Address common.Address `json:"address"`
	Percent int            `json:"percent"`
}

type MevBundleMetadata struct {
	// should be set only if we are receiving from Flashbots or other builders
	Signer *common.Address `json:"signer,omitempty"`
}

type MevSendBundleArgs struct {
	Version         string             `json:"version"`
	ReplacementUUID string             `json:"replacementUuid,omitempty"`
	Inclusion       MevBundleInclusion `json:"inclusion"`
	// when empty its considered cancel
	Body     []MevBundleBody    `json:"body"`
	Validity MevBundleValidity  `json:"validity"`
	Metadata *MevBundleMetadata `json:"metadata,omitempty"`

	// must be empty
	Privacy *json.RawMessage `json:"privacy,omitempty"`
}

// eth_sendRawTransaction

type EthSendRawTransactionArgs hexutil.Bytes

// eth_cancelBundle

type EthCancelBundleArgs struct {
	ReplacementUUID string          `json:"replacementUuid"`
	Signer          *common.Address `json:"signer,omitempty"`
	SigningAddress  *common.Address `json:"signingAddress"`
}

// bid_subsidiseBlock

type BidSubsisideBlockArgs uint64
