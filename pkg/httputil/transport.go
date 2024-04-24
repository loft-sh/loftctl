package httputil

import (
	"crypto/tls"
	"net/http"
)

func InsecureTransport() *http.Transport {
	newTransport := http.DefaultTransport.(*http.Transport).Clone()
	newTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return newTransport
}
