package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/flashbots/go-utils/rpcclient"
	"github.com/flashbots/go-utils/signature"
)

var DefaultOrderflowProxyPublicPort = 5544

var errCertificate = errors.New("failed to add certificate to pool")

func createTransportForSelfSignedCert(certPEM []byte) (*http.Transport, error) {
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(certPEM); !ok {
		return nil, errCertificate
	}
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: certPool,
		},
	}, nil
}

func RPCClientWithCertAndSigner(endpoint string, certPEM []byte, signer *signature.Signer) (rpcclient.RPCClient, error) {
	transport, err := createTransportForSelfSignedCert(certPEM)
	if err != nil {
		return nil, err
	}
	client := rpcclient.NewClientWithOpts(endpoint, &rpcclient.RPCClientOpts{
		HTTPClient: &http.Client{
			Transport: transport,
		},
		Signer: signer,
	})
	return client, nil
}

func OrderflowProxyURLFromIP(ip string) string {
	if strings.Contains(ip, ":") {
		return fmt.Sprintf("https://%s", ip)
	} else {
		return fmt.Sprintf("https://%s:%d", ip, DefaultOrderflowProxyPublicPort)
	}
}
