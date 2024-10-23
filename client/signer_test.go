package client

import (
	"testing"

	"github.com/flashbots/go-utils/signature"
	"github.com/stretchr/testify/assert"
)

func TestRequestSigner(t *testing.T) {
	check := assert.New(t)

	signer, err := RandomSigner()
	check.Nil(err)

	address := signer.Address()

	for _, body := range []string{"", "some body"} {
		body := []byte(body)
		header, err := signer.SignRequestBodyReturnHeader(body)
		check.Nil(err)

		recoveredAddress, err := signature.Verify(header, body)
		check.Nil(err)

		check.Equal(address, recoveredAddress)
	}
}
