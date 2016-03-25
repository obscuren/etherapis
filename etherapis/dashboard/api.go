// Contains the etherapis RESTful HTTP API endpoint.

package dashboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"

	"github.com/etherapis/etherapis/etherapis"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/mux"
	"gopkg.in/inconshreveable/log15.v2"
)

// api is a wrapper around the etherapis components, exposing various parts of
// each submodule.
type api struct {
	eapis *etherapis.EtherAPIs
}

// newAPIServeMux creates an etherapis API endpoint to serve RESTful requests,
// and returns the HTTP route multipelxer to embed in the main handler.
func newAPIServeMux(base string, eapis *etherapis.EtherAPIs) *mux.Router {
	// Create an API to expose various internals
	handler := &api{
		eapis: eapis,
	}
	// Register all the API handler endpoints
	router := mux.NewRouter()
	router.Handle(base, newStateServer(base, eapis))

	router.HandleFunc(base+"accounts", handler.Accounts)
	router.HandleFunc(base+"accounts/{address:0(x|X)[0-9a-fA-F]{40}}", handler.Account)
	router.HandleFunc(base+"accounts/{address:0(x|X)[0-9a-fA-F]{40}}/transactions", handler.Transactions)
	router.HandleFunc(base+"services/{address:0(x|X)[0-9a-fA-F]{40}}", handler.Services)
	router.HandleFunc(base+"subscriptions/{address}", handler.Subscriptions)
	router.HandleFunc(base+"subscriptions", handler.Subscriptions)

	return router
}

// Subscriptions retrieves the given address' subscriptions.
func (a *api) Subscriptions(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	switch {
	case r.Method == "GET":
		addr, exist := vars["address"]
		if !exist {
			log15.Error("failed to retrieve subscriptions", "error", "no address specified")
			http.Error(w, "no address specified", http.StatusInternalServerError)
			return
		}

		services, err := a.eapis.Contract().Subscriptions(common.HexToAddress(addr))
		if err != nil {
			log15.Error("failed to retrieve subscriptions", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		out, err := json.Marshal(services)
		if err != nil {
			log15.Error("failed to marshal subscriptions", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write(out)

	case r.Method == "POST":
		var (
			sender    = common.HexToAddress(r.FormValue("sender"))
			serviceId = common.String2Big(r.FormValue("serviceId"))
			api       = a.eapis.Contract()
		)
		log15.Info("creating new subscription", "sender", r.FormValue("sender"), "service-id", serviceId)

		key, err := a.eapis.GetPrivateKey(sender)
		if err != nil {
			log15.Error("failed retrieving sender key", "sender", sender, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		done := make(chan struct{})
		tx, err := api.Subscribe(key.PrivateKey, serviceId, new(big.Int).Mul(big.NewInt(10), common.Ether), func(sub *contract.Subscription) {
			defer close(done)

			out, err := json.Marshal(sub)
			if err != nil {
				log15.Error("failed to marshal subscription", "error", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Write(out)
		})
		if err != nil {
			close(done)

			log15.Error("failed to create transaction", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = a.eapis.SubmitTx(tx)
		if err != nil {
			close(done)

			log15.Error("failed to submit transaction", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// wait for subscription to be finished
		<-done
	}
}

// Accounts retrieves the Ethereum accounts currently configured to be used by the
// payment proxies and/or the marketplace and subscriptions. In case of a HTTP POST,
// a new account is imported using the uploaded key file and access password.
func (a *api) Accounts(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "POST" && r.FormValue("action") == "create":
		// Create a brand new random account
		address, err := a.eapis.CreateAccount()
		if err != nil {
			log15.Error("Failed to generate account", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte(fmt.Sprintf("0x%x", address)))

	case r.Method == "POST" && r.FormValue("action") == "import":
		// Import a new account specified by a form
		password := r.FormValue("password")
		account, _, err := r.FormFile("account")
		if err != nil {
			log15.Error("Failed to retrieve account", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		key, err := ioutil.ReadAll(account)
		if err != nil {
			log15.Error("Failed to read account", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		address, err := a.eapis.ImportAccount(key, password)
		if err != nil {
			log15.Error("Failed to import account", "error", err)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.Write([]byte(fmt.Sprintf("0x%x", address)))

	default:
		http.Error(w, "Unsupported method: "+r.Method, http.StatusMethodNotAllowed)
	}
}

// Account is the RESTfull endpoint for account management.
func (a *api) Account(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)

	// Only reply for deletion requests
	switch r.Method {
	case "GET":
		// Make sure the user provided a password to export with
		password := r.URL.Query().Get("password")
		if password == "" {
			log15.Error("Export with empty password denied", "address", params["address"])
			http.Error(w, "password required to export account", http.StatusBadRequest)
			return
		}
		// Export the key into a json key file and return and error if something goes wrong
		key, err := a.eapis.ExportAccount(common.HexToAddress(params["address"]), password)
		if err != nil {
			log15.Error("Failed to export account", "address", params["address"], "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Pretty print the key json since the exporter sucks :P
		pretty := new(bytes.Buffer)
		if err := json.Indent(pretty, key, "", "  "); err != nil {
			log15.Error("Failed to pretty print key", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Set the correct header to ensure download (i.e. no display) and dump the contents
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "inline; filename=\""+params["address"]+".json\"")
		w.Write(pretty.Bytes())

	case "DELETE":
		// Delete the account and return an error if something goes wrong
		if err := a.eapis.DeleteAccount(common.HexToAddress(params["address"])); err != nil {
			log15.Error("Failed to delete account", "address", params["address"], "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	default:
		log15.Error("Invalid method on account endpoint", "method", r.Method)
		http.Error(w, "Unsupported method: "+r.Method, http.StatusMethodNotAllowed)
		return
	}
}

// Transactions handles POST requests against an account endpoint to initiate
// outbound value transfers to other accounts.
func (a *api) Transactions(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)

	switch {
	case r.Method == "POST":
		// Parse and validate the transfer parameters
		sender := common.HexToAddress(params["address"])

		recipient := r.FormValue("recipient")
		if !common.IsHexAddress(recipient) {
			log15.Warn("Invalid recipient", "recipient", recipient)
			http.Error(w, fmt.Sprintf("Invalid recipient: %s", recipient), http.StatusBadRequest)
			return
		}
		to := common.HexToAddress(recipient)

		amount := r.FormValue("amount")
		value, ok := new(big.Int).SetString(amount, 10)
		if !ok || value.Cmp(big.NewInt(0)) <= 0 {
			log15.Warn("Invalid amount", "amount", amount)
			http.Error(w, fmt.Sprintf("Invalid amount: %s", amount), http.StatusBadRequest)
			return
		}
		// Execute the value transfer and return an error or the transaction id
		id, err := a.eapis.Transfer(sender, to, value)
		if err != nil {
			log15.Error("Failed to execute transfer", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write([]byte(fmt.Sprintf("0x%x", id)))

	default:
		http.Error(w, "Unsupported method: "+r.Method, http.StatusMethodNotAllowed)
	}
}

// Services returns the services for a given address or all services if
// no list of address is given.
func (a *api) Services(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)

	switch {
	case r.Method == "POST":
		// Create a brand new service based on parameters
		var (
			owner = common.HexToAddress(params["address"])
			name  = r.FormValue("name")
			url   = r.FormValue("url")
		)
		price, ok := new(big.Int).SetString(r.FormValue("price"), 10)
		if !ok {
			log15.Error("Invalid price for new service", "price", r.FormValue("price"))
			http.Error(w, fmt.Sprintf("Invalid price: %s", r.FormValue("price")), http.StatusBadRequest)
			return
		}
		cancel, ok := new(big.Int).SetString(r.FormValue("cancel"), 10)
		if !ok {
			log15.Error("Invalid cancellation time for new service", "time", r.FormValue("cancel"))
			http.Error(w, fmt.Sprintf("Invalid cancellation time: %s", r.FormValue("cancel")), http.StatusBadRequest)
			return
		}
		tx, err := a.eapis.CreateService(owner, name, url, price, cancel.Uint64())
		if err != nil {
			log15.Error("Failed to register service", "error", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Write([]byte(fmt.Sprintf("0x%x", tx.Hash())))

	default:
		http.Error(w, "Unsupported method: "+r.Method, http.StatusMethodNotAllowed)
	}
}
