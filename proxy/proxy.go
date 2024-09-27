package proxy

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

type Config struct {
	Log               *slog.Logger
	BuilderEndpoint   string
	ListenAddr        string
	ExternalAddr      string
	CertValidDuration time.Duration
	CertHosts         []string
	BuilderConfigHub  BuilderConfigHub
}

type Proxy struct {
	Config Config

	log                *slog.Logger
	orderflowSignerKey *ecdsa.PrivateKey
}

func New(config Config) (*Proxy, error) {
	return &Proxy{
		Config: config,
		log:    config.Log,
	}, nil
}

func (prx *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	_, err = http.Post(prx.Config.BuilderEndpoint, "content-type: application/json", bytes.NewBuffer(body))
	w.WriteHeader(http.StatusOK)
}

func (prx *Proxy) GenerateAndPublish() (tls.Certificate, error) {
	cert, key, err := GenerateCert(prx.Config.CertValidDuration, prx.Config.CertHosts)
	if err != nil {
		return tls.Certificate{}, err
	}

	orderflowSignerKey, err := crypto.GenerateKey()
	if err != nil {
		return tls.Certificate{}, err
	}
	prx.orderflowSignerKey = orderflowSignerKey
	orderflowSigner := crypto.PubkeyToAddress(orderflowSignerKey.PublicKey)

	prx.log.Info("Generated ordeflow signer", "address", orderflowSigner)

	selfInfo := BuilderInfo{
		Cert:            cert,
		OrderflowSigner: orderflowSigner,
		NetworkAddress:  prx.Config.ExternalAddr,
	}

	err = prx.Config.BuilderConfigHub.PublishConfig(selfInfo)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.X509KeyPair(cert, key)
}

func (prx *Proxy) RunProxyInBackground() error {
	certificate, err := prx.GenerateAndPublish()
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:    prx.Config.ListenAddr,
		Handler: prx,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{certificate},
		},
	}
	go func() {
		prx.log.Info("Starting orderflow proxy", "addr", srv.Addr)
		if err := srv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			prx.log.Error("Orderflow proxy failed", "err", err)
		}
	}()
	return nil
}
