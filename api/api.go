// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package api

import (
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/api/debug"
	"github.com/vechain/thor/v2/api/doc"
	"github.com/vechain/thor/v2/api/events"
	"github.com/vechain/thor/v2/api/node"
	"github.com/vechain/thor/v2/api/subscriptions"
	"github.com/vechain/thor/v2/api/transactions"
	"github.com/vechain/thor/v2/api/transfers"
	"github.com/vechain/thor/v2/chain"
	"github.com/vechain/thor/v2/logdb"
	"github.com/vechain/thor/v2/state"
	"github.com/vechain/thor/v2/telemetry"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/txpool"
)

// New return api router
func New(
	repo *chain.Repository,
	stater *state.Stater,
	txPool *txpool.TxPool,
	logDB *logdb.LogDB,
	bft blocks.BFTEngine,
	nw node.Network,
	allowedOrigins string,
	backtraceLimit uint32,
	callGasLimit uint64,
	pprofOn bool,
	skipLogs bool,
	allowCustomTracer bool,
	forkConfig thor.ForkConfig,
	enableMetrics bool,
) (http.HandlerFunc, func()) {
	origins := strings.Split(strings.TrimSpace(allowedOrigins), ",")
	for i, o := range origins {
		origins[i] = strings.ToLower(strings.TrimSpace(o))
	}

	router := mux.NewRouter()

	// to serve api docs
	if enableMetrics {
		router.PathPrefix("/node/metrics").Handler(telemetry.Handler())
	}

	// to serve api doc and swagger-ui
	router.PathPrefix("/doc").Handler(
		http.StripPrefix("/doc/", http.FileServer(http.FS(doc.FS))),
	)

	// redirect stoplight-ui
	router.Path("/").HandlerFunc(
		func(w http.ResponseWriter, req *http.Request) {
			http.Redirect(w, req, "doc/stoplight-ui/", http.StatusTemporaryRedirect)
		})

	accounts.New(repo, stater, callGasLimit, forkConfig).
		Mount(router, "/accounts")

	if !skipLogs {
		events.New(repo, logDB).
			Mount(router, "/logs/event")
		transfers.New(repo, logDB).
			Mount(router, "/logs/transfer")
	}
	blocks.New(repo, bft).
		Mount(router, "/blocks")
	transactions.New(repo, txPool).
		Mount(router, "/transactions")
	debug.New(repo, stater, forkConfig, callGasLimit, allowCustomTracer).
		Mount(router, "/debug")
	node.New(nw).
		Mount(router, "/node")
	subs := subscriptions.New(repo, origins, backtraceLimit, txPool)
	subs.Mount(router, "/subscriptions")

	if pprofOn {
		router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		router.HandleFunc("/debug/pprof/profile", pprof.Profile)
		router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		router.HandleFunc("/debug/pprof/trace", pprof.Trace)
		router.PathPrefix("/debug/pprof/").HandlerFunc(pprof.Index)
	}

	handler := handlers.CompressHandler(router)
	handler = handlers.CORS(
		handlers.AllowedOrigins(origins),
		handlers.AllowedHeaders([]string{"content-type", "x-genesis-id"}),
		handlers.ExposedHeaders([]string{"x-genesis-id", "x-thorest-ver"}),
	)(handler)
	return handler.ServeHTTP,
		subs.Close // subscriptions handles hijacked conns, which need to be closed
}
