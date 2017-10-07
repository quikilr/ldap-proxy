// Copyright © 2017 Stefan Kollmann
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package pkg

import (
	"errors"
	"fmt"
	"github.com/kolleroot/ldap-proxy/pkg/log"
	"github.com/samuel/go-ldap/ldap"
	"net"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	errInvalidSessionType = errors.New("proxy: Invalid session type")
)

var (
	requestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Subsystem: "proxy",
		Name:      "requests_total",
		Help:      "The total number of requests",
	}, []string{"action"})

	backendActionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: "proxy",
		Name:      "backend_duration",
		Help:      "The time spent by the backend searching",
		Buckets:   []float64{.0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
	}, []string{"action", "backend"})
)

func init() {
	prometheus.MustRegister(requestsTotal)
	prometheus.MustRegister(backendActionDuration)
}

type LdapProxy struct {
	backends map[string]Backend

	server *ldap.Server
}

type session struct {
	dn string
}

func (session *session) LogAuth(dn string, successful bool) {
	if successful {
		log.Printf("%s: Authentication successful", dn)
	} else {
		log.Printf("%s: Authentication failed", dn)
	}
}

func (session *session) Println(v ...interface{}) {
	log.Printf("%s: %s", session.dn, fmt.Sprint(v...))
}

func (session *session) Printf(format string, v ...interface{}) {
	log.Printf("%s: %s", session.dn, fmt.Sprintf(format, v...))
}

func NewLdapProxy() *LdapProxy {
	proxy := &LdapProxy{
		backends: make(map[string]Backend),
		server:   &ldap.Server{},
	}

	proxy.server.Backend = proxy

	return proxy
}

func (proxy *LdapProxy) AddBackend(backends ...Backend) {
	log.Printf("Adding %d backends", len(backends))
	for _, bkend := range backends {
		proxy.backends[bkend.Name()] = bkend
	}
}

func (proxy *LdapProxy) ListenAndServe(addr string) {
	log.Printf("Start listening on %s", addr)
	proxy.server.Serve("tcp", addr)
}

func (serverBackend *LdapProxy) Connect(remoteAddr net.Addr) (ldap.Context, error) {
	log.Printf("New session from %v", remoteAddr)

	requestsTotal.With(prometheus.Labels{"action": "connect"}).Inc()

	return &session{}, nil
}

func (serverBackend *LdapProxy) Disconnect(ctx ldap.Context) {
	sess, ok := ctx.(*session)
	if !ok {
		return
	}

	requestsTotal.With(prometheus.Labels{"action": "disconnect"}).Inc()

	sess.Println("Session ended")
}

func (serverBackend *LdapProxy) Bind(ctx ldap.Context, req *ldap.BindRequest) (*ldap.BindResponse, error) {
	log.Debugf("bind as %s", req.DN)

	sess, ok := ctx.(*session)
	if !ok {
		return nil, errInvalidSessionType
	}

	requestsTotal.With(prometheus.Labels{"action": "bind"}).Inc()

	res := &ldap.BindResponse{
		BaseResponse: ldap.BaseResponse{
			Code: ldap.ResultInvalidCredentials,
		},
	}

	sess.dn = ""

	for _, backend := range serverBackend.backends {
		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			backendActionDuration.With(prometheus.Labels{"action": "auth", "backend": backend.Name()}).Observe(v)
		}))
		authenticated := backend.Authenticate(req.DN, string(req.Password))
		timer.ObserveDuration()

		if authenticated {
			sess.dn = req.DN

			res.BaseResponse.Code = ldap.ResultSuccess
			res.MatchedDN = req.DN
			break
		}
	}

	sess.LogAuth(req.DN, res.BaseResponse.Code == ldap.ResultSuccess)

	return res, nil
}

func (serverBackend *LdapProxy) Add(ctx ldap.Context, req *ldap.AddRequest) (*ldap.AddResponse, error) {
	requestsTotal.With(prometheus.Labels{"action": "add"}).Inc()

	return &ldap.AddResponse{
		BaseResponse: ldap.BaseResponse{
			Code: ldap.ResultUnwillingToPerform,
		},
	}, nil
}

func (serverBackend *LdapProxy) Delete(ctx ldap.Context, req *ldap.DeleteRequest) (*ldap.DeleteResponse, error) {
	requestsTotal.With(prometheus.Labels{"action": "delete"}).Inc()

	return &ldap.DeleteResponse{
		BaseResponse: ldap.BaseResponse{
			Code: ldap.ResultUnwillingToPerform,
		},
	}, nil
}

func (serverBackend *LdapProxy) ExtendedRequest(ctx ldap.Context, req *ldap.ExtendedRequest) (*ldap.ExtendedResponse, error) {
	requestsTotal.With(prometheus.Labels{"action": "extended"}).Inc()

	return &ldap.ExtendedResponse{
		BaseResponse: ldap.BaseResponse{
			Code: ldap.ResultUnwillingToPerform,
		},
	}, nil
}

func (serverBackend *LdapProxy) Modify(ctx ldap.Context, req *ldap.ModifyRequest) (*ldap.ModifyResponse, error) {
	requestsTotal.With(prometheus.Labels{"action": "modify"}).Inc()

	return &ldap.ModifyResponse{
		BaseResponse: ldap.BaseResponse{
			Code: ldap.ResultUnwillingToPerform,
		},
	}, nil
}

func (serverBackend *LdapProxy) ModifyDN(ctx ldap.Context, req *ldap.ModifyDNRequest) (*ldap.ModifyDNResponse, error) {
	requestsTotal.With(prometheus.Labels{"action": "modify_dn"}).Inc()

	return &ldap.ModifyDNResponse{
		BaseResponse: ldap.BaseResponse{
			Code: ldap.ResultUnwillingToPerform,
		},
	}, nil
}

func (serverBackend *LdapProxy) PasswordModify(ctx ldap.Context, req *ldap.PasswordModifyRequest) ([]byte, error) {
	requestsTotal.With(prometheus.Labels{"action": "modify_password"}).Inc()

	return []byte{}, nil
}

func (serverBackend *LdapProxy) Search(ctx ldap.Context, req *ldap.SearchRequest) (*ldap.SearchResponse, error) {
	sess, ok := ctx.(*session)
	if !ok {
		return nil, errInvalidSessionType
	}

	requestsTotal.With(prometheus.Labels{"action": "search"}).Inc()

	if sess.dn == "" {
		return &ldap.SearchResponse{
			BaseResponse: ldap.BaseResponse{
				Code: ldap.ResultInsufficientAccessRights,
			},
		}, nil
	}

	sess.Printf("Searching dn: '%s', filter: '%s'", req.BaseDN, req.Filter)

	res := &ldap.SearchResponse{
		BaseResponse: ldap.BaseResponse{
			Code: ldap.ResultSuccess,
		},
	}

	var searchResults []*ldap.SearchResult

	for _, backend := range serverBackend.backends {
		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			backendActionDuration.With(prometheus.Labels{"action": "search", "backend": backend.Name()}).Observe(v)
		}))
		users, err := backend.GetUsers(req.Filter)
		timer.ObserveDuration()
		if err != nil {
			return nil, err
		}

		for _, user := range users {
			searchResult := ldap.SearchResult{
				DN:         user.DN,
				Attributes: map[string][][]byte{},
			}

			for key, values := range user.Attributes {
				convertedValues := [][]byte{}
				for _, value := range values {
					convertedValues = append(convertedValues, []byte(value))
				}
				searchResult.Attributes[key] = convertedValues
			}

			searchResults = append(searchResults, &searchResult)
		}
	}

	res.Results = searchResults

	return res, nil
}

func (serverBackend *LdapProxy) Whoami(ctx ldap.Context) (string, error) {
	sess, ok := ctx.(*session)
	if !ok {
		return "", errInvalidSessionType
	}

	requestsTotal.With(prometheus.Labels{"action": "whoami"}).Inc()

	sess.Println("Who am I")

	return sess.dn, nil
}
