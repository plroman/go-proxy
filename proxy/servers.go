package proxy

import (
	"errors"
	"log/slog"
	"net/http"
	"time"
)

var (
	HTTPDefaultReadTimeout  = 60 * time.Second
	HTTPDefaultWriteTimeout = 30 * time.Second
)

type OrderflowProxyServers struct {
	proxy        *Proxy
	publicServer *http.Server
	localServer  *http.Server
	certServer   *http.Server
}

func StartServers(proxy *Proxy, publicListenAddress, localListenAddress, certListenAddress string) (*OrderflowProxyServers, error) {
	publicServer := &http.Server{
		Addr:         publicListenAddress,
		Handler:      proxy.PublicHandler,
		TLSConfig:    proxy.TLSConfig(),
		ReadTimeout:  HTTPDefaultReadTimeout,
		WriteTimeout: HTTPDefaultWriteTimeout,
	}
	localServer := &http.Server{
		Addr:         localListenAddress,
		Handler:      proxy.LocalHandler,
		TLSConfig:    proxy.TLSConfig(),
		ReadTimeout:  HTTPDefaultReadTimeout,
		WriteTimeout: HTTPDefaultWriteTimeout,
	}
	certServer := &http.Server{
		Addr:         certListenAddress,
		Handler:      proxy.CertHandler,
		ReadTimeout:  HTTPDefaultReadTimeout,
		WriteTimeout: HTTPDefaultWriteTimeout,
	}

	errCh := make(chan error)

	go func() {
		if err := publicServer.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			err = errors.Join(errors.New("public HTTP server failed"), err)
			errCh <- err
		}
	}()
	go func() {
		if err := localServer.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			err = errors.Join(errors.New("local HTTP server failed"), err)
			errCh <- err
		}
	}()
	go func() {
		if err := certServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			err = errors.Join(errors.New("cert HTTP server failed"), err)
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return nil, err
	case <-time.After(time.Millisecond * 100):
	}

	go func() {
		for {
			err, more := <-errCh
			if !more {
				return
			}
			proxy.Log.Error("Error in HTTP server", slog.Any("error", err))
		}
	}()

	return &OrderflowProxyServers{
		proxy:        proxy,
		publicServer: publicServer,
		localServer:  localServer,
		certServer:   certServer,
	}, nil
}

func (s *OrderflowProxyServers) Stop() {
	_ = s.publicServer.Close()
	_ = s.localServer.Close()
	_ = s.certServer.Close()
	s.proxy.Stop()
}
