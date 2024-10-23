package client

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

type RequestSigner struct {
	privateKey *ecdsa.PrivateKey
	address    common.Address
	hexAddress string
}

func NewRequestSigner(privateKey *ecdsa.PrivateKey) RequestSigner {
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	return RequestSigner{
		privateKey: privateKey,
		hexAddress: address.Hex(),
		address:    address,
	}
}

func RandomSigner() (*RequestSigner, error) {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	signer := NewRequestSigner(privateKey)
	return &signer, nil
}

func (s *RequestSigner) Address() common.Address {
	return s.address
}

const SignatureHeader = "X-Flashbots-Signature"

func (s *RequestSigner) SignRequestBodyReturnHeader(body []byte) (string, error) {
	hashedBody := crypto.Keccak256Hash([]byte(body)).Hex()
	sig, err := crypto.Sign(accounts.TextHash([]byte(hashedBody)), s.privateKey)
	if err != nil {
		return "", err
	}

	signature := s.hexAddress + ":" + hexutil.Encode(sig)

	return signature, nil
}
