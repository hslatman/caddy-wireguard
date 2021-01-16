// Copyright 2021 Herman Slatman
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package wireguard

import (
	b64 "encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"

	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

func init() {
	caddy.RegisterModule(WireGuard{})
}

// CaddyModule returns the Caddy module information.
func (WireGuard) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "wireguard",
		New: func() caddy.Module { return new(WireGuard) },
	}
}

// WireGuard is an App that ... ;-)
type WireGuard struct {
	ctx     caddy.Context
	logger  *zap.Logger
	httpApp *caddyhttp.App
}

// Provision sets up the WireGuard app.
func (w *WireGuard) Provision(ctx caddy.Context) error {

	// store some references
	httpAppIface, err := ctx.App("http")
	if err != nil {
		return fmt.Errorf("getting http app: %v", err)
	}
	w.httpApp = httpAppIface.(*caddyhttp.App)

	fmt.Println(w.httpApp.Servers)
	for n, s := range w.httpApp.Servers {
		fmt.Println(fmt.Sprintf("%s - %#+v", n, s))
	}

	w.ctx = ctx
	w.logger = ctx.Logger(w)
	defer w.logger.Sync()

	return nil
}

// Validate ensures the app's configuration is valid.
func (w *WireGuard) Validate() error {
	return nil
}

// Start starts the CrowdSec Caddy app
func (w *WireGuard) Start() error {
	tun, tnet, err := tun.CreateNetTUN(
		[]net.IP{net.ParseIP("192.168.31.38")},
		[]net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("8.8.4.4")},
		1420,
	)
	if err != nil {
		w.logger.Error(err.Error())
		return err
	}

	fmt.Println(tun, tnet)
	fmt.Println(fmt.Sprintf("%#+v", tun))
	fmt.Println(fmt.Sprintf("%#+v", tnet))

	logger := log.New(os.Stderr, "", log.LstdFlags)

	// [Interface]
	// PrivateKey = 6M8iJ4VMoDpdY3fLw3HEvxqy+9K2Lj6lypGBVx7ooHc=
	// Address = 192.168.4.6/24
	// DNS = 8.8.8.8, 8.8.4.4, 1.1.1.1, 1.0.0.1

	// [Peer]
	// PublicKey = JRI8Xc0zKP9kXk8qP84NdUQA04h6DLfFbwJn4g+/PFs=
	// Endpoint = demo.wireguard.com:12912
	// AllowedIPs = 0.0.0.0/0

	publicKeyB64 := "k6z61BBVP8HOyRs63O+TP8SsR936tD3THq0Cpxj+FlE="
	privateKeyB64 := "6M8iJ4VMoDpdY3fLw3HEvxqy+9K2Lj6lypGBVx7ooHc="

	publicKey, _ := b64.StdEncoding.DecodeString(publicKeyB64)
	privateKey, _ := b64.StdEncoding.DecodeString(privateKeyB64)

	publicKeyHex := hex.EncodeToString(publicKey)
	privateKeyHex := hex.EncodeToString(privateKey)

	fmt.Println(publicKeyHex, privateKeyHex)

	// config := fmt.Sprintf(`
	// 	private_key=%s
	// 	public_key=%s
	// 	endpoint=demo.wireguard.com:12912
	// 	allowed_ip=0.0.0.0/0
	// 	persistent_keepalive_interval=25
	// `, privateKeyHex, publicKeyHex)

	// NOTE: format of the below is SUPER important; it breaks stuff if it isn't correct!
	// 	config := fmt.Sprintf(`private_key=%s
	// listen_port=51820
	// public_key=%s
	// allowed_ip=0.0.0.0/0
	// persistent_keepalive_interval=25
	// `, privateKeyHex, publicKeyHex)

	listenPort := 51820

	config := fmt.Sprintf(`private_key=%s
listen_port=%d
public_key=%s
allowed_ip=0.0.0.0/0
`, privateKeyHex, listenPort, publicKeyHex)

	fmt.Println(config)

	dev := device.NewDevice(tun, &device.Logger{logger, logger, logger})
	dev.IpcSet(config)
	dev.Up()
	// TODO: mapping from the Caddy listeners to listeners here?
	// Then do http/l4 proxying?

	fmt.Println(w.httpApp.Servers)
	for n, s := range w.httpApp.Servers {
		//fmt.Println(fmt.Sprintf("%s - %#+v", n, s))
		fmt.Println(fmt.Sprintf("serving: %s", n))

		if n == "remaining_auto_https_redirects" {
			continue
		}

		port, _ := strconv.Atoi(strings.Split(s.Listen[0], ":")[1])
		listener, err := tnet.ListenTCP(&net.TCPAddr{Port: port})
		if err != nil {
			w.logger.Error(err.Error())
		}

		http.HandleFunc("/", s.ServeHTTP)

		// http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		// 	w.logger.Info(fmt.Sprintf("> %s - %s - %s", request.RemoteAddr, request.URL.String(), request.UserAgent()))
		// 	io.WriteString(writer, "Hello from userspace TCP!")
		// })

		//s.ServeHTTP()
		go func() {
			err = http.Serve(listener, nil)
			if err != nil {
				w.logger.Error(err.Error())
			}
		}()

	}

	// 	logger := log.New(os.Stderr, "", log.LstdFlags)

	// 	tun, tnet, err := tun.CreateNetTUN(
	// 		[]net.IP{net.ParseIP("192.168.4.29")},
	// 		[]net.IP{net.ParseIP("8.8.8.8")},
	// 		1420)
	// 	if err != nil {
	// 		log.Panic(err)
	// 	}
	// 	dev := device.NewDevice(tun, &device.Logger{logger, logger, logger})
	// 	dev.IpcSet(`private_key=a8dac1d8a70a751f0f699fb14ba1cff7b79cf4fbd8f09f44c6e6a90d0369604f
	// public_key=25123c5dcd3328ff645e4f2a3fce0d754400d3887a0cb7c56f0267e20fbf3c5b
	// endpoint=163.172.161.0:12912
	// allowed_ip=0.0.0.0/0
	// `)
	// 	dev.Up()

	// 	client := http.Client{
	// 		Transport: &http.Transport{
	// 			DialContext: tnet.DialContext,
	// 		},
	// 	}
	// 	resp, err := client.Get("https://www.zx2c4.com/ip")
	// 	if err != nil {
	// 		log.Panic(err)
	// 	}
	// 	body, err := io.ReadAll(resp.Body)
	// 	if err != nil {
	// 		log.Panic(err)
	// 	}
	// 	log.Println(string(body))

	return nil
}

// Stop stops the CrowdSec Caddy app
func (w *WireGuard) Stop() error {

	return nil
}

// Interface guards
var (
	_ caddy.Module      = (*WireGuard)(nil)
	_ caddy.App         = (*WireGuard)(nil)
	_ caddy.Provisioner = (*WireGuard)(nil)
	_ caddy.Validator   = (*WireGuard)(nil)
)
