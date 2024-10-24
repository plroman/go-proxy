// Package proxy provides the main proxy server.
package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
)

type ConfighubOrderflowProxyCredentials struct {
	TLSCert            string         `json:"tls_cert"`
	EcdsaPubkeyAddress common.Address `json:"ecdsa_pubkey_address"`
}

type ConfighubBuilder struct {
	IP             string                             `json:"ip"`
	OrderflowProxy ConfighubOrderflowProxyCredentials `json:"orderflow_proxy"`
}

type BuilderConfigHub interface {
	RegisterCredentials(args ConfighubOrderflowProxyCredentials) error
	Builders() ([]ConfighubBuilder, error)
}

type MockBuilderConfigHub struct{}

func (m MockBuilderConfigHub) RegisterCredentials(info ConfighubOrderflowProxyCredentials) error {
	return nil
}

func (m MockBuilderConfigHub) Builders() ([]ConfighubBuilder, error) {
	return nil, nil
}

type builderConfigHubImpl struct {
	endpoint string
}

func NewBuilderConfigHub(endpoint string) BuilderConfigHub {
	return &builderConfigHubImpl{
		endpoint: endpoint,
	}
}

func (b *builderConfigHubImpl) RegisterCredentials(info ConfighubOrderflowProxyCredentials) error {
	body, err := json.Marshal(info)
	if err != nil {
		return err
	}
	resp, err := http.Post(b.endpoint+"/api/l1-builder/v1/register_credentials/orderflow-proxy", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("builder config hub returned error, code: %d, body: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (b *builderConfigHubImpl) Builders() ([]ConfighubBuilder, error) {
	resp, err := http.Get(b.endpoint + "/api/l1-builder/v1/builders")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result []ConfighubBuilder
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
