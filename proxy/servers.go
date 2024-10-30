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
	proxy         *Proxy
	networkServer *http.Server
	userServer    *http.Server
	certServer    *http.Server
}

func StartServers(proxy *Proxy, networkListenAddress, userListenAddress, certListenAddress string) (*OrderflowProxyServers, error) {
	networkServer := &http.Server{
		Addr:         networkListenAddress,
		Handler:      proxy.PublicHandler,
		TLSConfig:    proxy.TLSConfig(),
		ReadTimeout:  HTTPDefaultReadTimeout,
		WriteTimeout: HTTPDefaultWriteTimeout,
	}
	userServer := &http.Server{
		Addr:         userListenAddress,
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
		if err := networkServer.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			err = errors.Join(errors.New("network HTTP server failed"), err)
			errCh <- err
		}
	}()
	go func() {
		if err := userServer.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			err = errors.Join(errors.New("user HTTP server failed"), err)
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
		proxy:         proxy,
		networkServer: networkServer,
		userServer:    userServer,
		certServer:    certServer,
	}, nil
}

func (s *OrderflowProxyServers) Stop() {
	_ = s.networkServer.Close()
	_ = s.userServer.Close()
	_ = s.certServer.Close()
	s.proxy.Stop()
}
