package main

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"peerswap-web/cmd/psweb/config"
	"peerswap-web/cmd/psweb/ln"
	"peerswap-web/cmd/psweb/mempool"
	"peerswap-web/cmd/psweb/ps"

	"github.com/elementsproject/peerswap/peerswaprpc"
	"github.com/gorilla/mux"
)

type AliasCache struct {
	PublicKey string
	Alias     string
}

var (
	aliasCache []AliasCache
	templates  = template.New("")
	//go:embed static
	staticFiles embed.FS
	//go:embed templates/*.gohtml
	tplFolder embed.FS
	logFile   *os.File
)

const version = "v1.2.6"

func main() {

	var (
		dataDir     = flag.String("datadir", "", "Path to config folder")
		showHelp    = flag.Bool("help", false, "Show help")
		showVersion = flag.Bool("version", false, "Show version")
	)

	flag.Parse()

	if *showHelp {
		showHelpMessage()
		return
	}

	if *showVersion {
		showVersionInfo()
		return
	}

	// loading from the config file or assigning defaults
	config.Load(*dataDir)

	// save config to confirm any defaults
	config.Save()

	// set logging params
	err := setLogging()
	if err != nil {
		log.Fatal(err)
	}

	// Get all HTML template files from the embedded filesystem
	templateFiles, err := tplFolder.ReadDir("templates")
	if err != nil {
		log.Fatal(err)
	}

	// Store template names
	var templateNames []string
	for _, file := range templateFiles {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".gohtml" {
			templateNames = append(templateNames, filepath.Join("templates", file.Name()))
		}
	}

	// Parse all template files in the templates directory
	templates = template.Must(templates.
		Funcs(template.FuncMap{
			"sats": toSats,
			"u":    toUint,
			"fmt":  formatWithThousandSeparators,
			"m":    toMil,
		}).
		ParseFS(tplFolder, templateNames...))

	// create an embedded Filesystem
	var staticFS = http.FS(staticFiles)
	fs := http.FileServer(staticFS)

	// Serve static files
	http.Handle("/static/", fs)

	r := mux.NewRouter()

	// Serve templates
	r.HandleFunc("/", indexHandler)
	r.HandleFunc("/swap", swapHandler)
	r.HandleFunc("/peer", peerHandler)
	r.HandleFunc("/submit", submitHandler)
	r.HandleFunc("/save", saveConfigHandler)
	r.HandleFunc("/config", configHandler)
	r.HandleFunc("/stop", stopHandler)
	r.HandleFunc("/update", updateHandler)
	r.HandleFunc("/liquid", liquidHandler)
	r.HandleFunc("/loading", loadingHandler)
	r.HandleFunc("/log", logHandler)
	r.HandleFunc("/logapi", logApiHandler)
	r.HandleFunc("/backup", backupHandler)
	r.HandleFunc("/bitcoin", bitcoinHandler)
	r.HandleFunc("/pegin", peginHandler)
	r.HandleFunc("/bumpfee", bumpfeeHandler)

	// Start the server
	http.Handle("/", r)
	go func() {
		if err := http.ListenAndServe(":"+config.Config.ListenPort, nil); err != nil {
			log.Fatal(err)
		}
	}()

	log.Println("Listening on http://localhost:" + config.Config.ListenPort)

	// Start Telegram bot
	go telegramStart()

	// Run every minute
	go startTimer()

	// Handle termination signals
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for termination signal
	<-signalChan
	log.Println("Received termination signal")

	// close log
	closeLogFile()

	// Exit the program gracefully
	os.Exit(0)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {

	if config.Config.ElementsPass == "" || config.Config.ElementsUser == "" {
		http.Redirect(w, r, "/config?err=welcome", http.StatusSeeOther)
		return
	}

	// this method will fail if peerswap is not running or misconfigured
	res, err := ps.ReloadPolicyFile()
	if err != nil {
		redirectWithError(w, r, "/config?", err)
		return
	}

	allowlistedPeers := res.GetAllowlistedPeers()
	suspiciousPeers := res.GetSuspiciousPeerList()

	res2, err := ps.ListPeers()
	if err != nil {
		redirectWithError(w, r, "/config?", err)
		return
	}
	peers := res2.GetPeers()

	res3, err := ps.ListSwaps()
	if err != nil {
		redirectWithError(w, r, "/config?", err)
		return
	}
	swaps := res3.GetSwaps()

	res4, err := ps.LiquidGetBalance()
	if err != nil {
		redirectWithError(w, r, "/config?", err)
		return
	}

	satAmount := res4.GetSatAmount()

	//check for error message to display
	message := ""
	keys, ok := r.URL.Query()["err"]
	if ok && len(keys[0]) > 0 {
		message = keys[0]
	}

	type Page struct {
		AllowSwapRequests bool
		BitcoinSwaps      bool
		Message           string
		ColorScheme       string
		LiquidBalance     uint64
		ListPeers         string
		ListSwaps         string
		BitcoinBalance    uint64
	}

	data := Page{
		AllowSwapRequests: config.Config.AllowSwapRequests,
		BitcoinSwaps:      config.Config.BitcoinSwaps,
		Message:           message,
		ColorScheme:       config.Config.ColorScheme,
		LiquidBalance:     satAmount,
		ListPeers:         convertPeersToHTMLTable(peers, allowlistedPeers, suspiciousPeers),
		ListSwaps:         convertSwapsToHTMLTable(swaps),
		BitcoinBalance:    uint64(ln.ConfirmedWalletBalance()),
	}

	// executing template named "homepage"
	err = templates.ExecuteTemplate(w, "homepage", data)
	if err != nil {
		log.Fatalln(err)
		http.Error(w, http.StatusText(500), 500)
	}
}

func peerHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["id"]
	if !ok || len(keys[0]) < 1 {
		fmt.Println("URL parameter 'id' is missing")
		http.Error(w, http.StatusText(500), 500)
		return
	}

	id := keys[0]

	res, err := ps.ListPeers()
	if err != nil {
		log.Printf("unable to connect to RPC server: %v", err)
		redirectWithError(w, r, "/config?", err)
		return
	}
	peers := res.GetPeers()
	peer := findPeerById(peers, id)

	if peer == nil {
		log.Printf("unable to find peer by id: %v", id)
		redirectWithError(w, r, "/config?", errors.New("unable to find peer by id"))
		return
	}

	var sumLocal uint64
	var sumRemote uint64
	for _, ch := range peer.Channels {
		sumLocal += ch.LocalBalance
		sumRemote += ch.RemoteBalance
	}

	res2, err := ps.ReloadPolicyFile()
	if err != nil {
		log.Printf("unable to connect to RPC server: %v", err)
		redirectWithError(w, r, "/config?", err)
		return
	}
	allowlistedPeers := res2.GetAllowlistedPeers()
	suspiciousPeers := res2.GetSuspiciousPeerList()

	res3, err := ps.LiquidGetBalance()
	if err != nil {
		log.Printf("unable to connect to RPC server: %v", err)
		redirectWithError(w, r, "/config?", err)
		return
	}

	satAmount := res3.GetSatAmount()

	res4, err := ps.ListActiveSwaps()
	if err != nil {
		redirectWithError(w, r, "/config?", err)
		return
	}

	activeSwaps := res4.GetSwaps()

	//check for error message to display
	message := ""
	keys, ok = r.URL.Query()["err"]
	if ok && len(keys[0]) > 0 {
		message = keys[0]
	}

	type Page struct {
		Message        string
		ColorScheme    string
		Peer           *peerswaprpc.PeerSwapPeer
		PeerAlias      string
		NodeUrl        string
		Allowed        bool
		Suspicious     bool
		LBTC           bool
		BTC            bool
		LiquidBalance  uint64
		BitcoinBalance uint64
		ActiveSwaps    string
		DirectionIn    bool
	}

	data := Page{
		Message:        message,
		ColorScheme:    config.Config.ColorScheme,
		Peer:           peer,
		PeerAlias:      getNodeAlias(peer.NodeId),
		NodeUrl:        config.Config.NodeApi,
		Allowed:        stringIsInSlice(peer.NodeId, allowlistedPeers),
		Suspicious:     stringIsInSlice(peer.NodeId, suspiciousPeers),
		BTC:            stringIsInSlice("btc", peer.SupportedAssets),
		LBTC:           stringIsInSlice("lbtc", peer.SupportedAssets),
		LiquidBalance:  satAmount,
		BitcoinBalance: uint64(ln.ConfirmedWalletBalance()),
		ActiveSwaps:    convertSwapsToHTMLTable(activeSwaps),
		DirectionIn:    sumLocal < sumRemote,
	}

	// executing template named "peer"
	err = templates.ExecuteTemplate(w, "peer", data)
	if err != nil {
		log.Fatalln(err)
		http.Error(w, http.StatusText(500), 500)
	}
}

func swapHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["id"]
	if !ok || len(keys[0]) < 1 {
		fmt.Println("URL parameter 'id' is missing")
		http.Error(w, http.StatusText(500), 500)
		return
	}
	id := keys[0]

	type Page struct {
		ColorScheme string
		Id          string
		Message     string
	}

	data := Page{
		ColorScheme: config.Config.ColorScheme,
		Id:          id,
		Message:     "",
	}

	// executing template named "swap"
	err := templates.ExecuteTemplate(w, "swap", data)
	if err != nil {
		log.Fatalln(err)
		http.Error(w, http.StatusText(500), 500)
	}
}

// Updates swap page live
func updateHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["id"]
	if !ok || len(keys[0]) < 1 {
		fmt.Println("URL parameter 'id' is missing")
		http.Error(w, http.StatusText(500), 500)
		return
	}

	id := keys[0]

	res, err := ps.GetSwap(id)
	if err != nil {
		log.Printf("onSwap: %v", err)
		redirectWithError(w, r, "/swap?id="+id+"&", err)
		return
	}

	swap := res.GetSwap()

	url := config.Config.BitcoinApi + "/tx/"
	if swap.Asset == "lbtc" {
		url = config.Config.LiquidApi + "/tx/"
	}
	swapData := `<div class="container">
	<div class="columns">
	  <div class="column">
		<div class="box">
		  <table style="table-layout:fixed; width: 100%;">
				<tr>
			  <td style="float: left; text-align: left; width: 80%;">
				<h3 class="title is-4">Swap Details</h3>
			  </td>
			  </td><td style="float: right; text-align: right; width:20%;">
				<h3 class="title is-4">`
	swapData += visualiseSwapStatus(swap.State, true)
	swapData += `</h3>
			  </td>
			</tr>
		  <table>
		  <table style="table-layout:fixed; width: 100%">
			<tr><td style="width:30%; text-align: right">ID:</td><td style="overflow-wrap: break-word;">`
	swapData += swap.Id
	swapData += `</td></tr>
			<tr><td style="text-align: right">Created At:</td><td >`
	swapData += time.Unix(swap.CreatedAt, 0).UTC().Format("2006-01-02 15:04:05")
	swapData += `</td></tr>
			<tr><td style="text-align: right">Asset:</td><td>`
	swapData += swap.Asset
	swapData += `</td></tr>
			<tr><td style="text-align: right">Type:</td><td>`
	swapData += swap.Type
	swapData += `</td></tr>
			<tr><td style="text-align: right">Role:</td><td>`
	swapData += swap.Role
	swapData += `</td></tr>
			<tr><td style="text-align: right">State:</td><td style="overflow-wrap: break-word;">`
	swapData += swap.State
	swapData += `</td></tr>
			<tr><td style="text-align: right">Initiator:</td><td style="overflow-wrap: break-word;">`
	swapData += getNodeAlias(swap.InitiatorNodeId)
	swapData += `&nbsp<a href="`
	swapData += config.Config.NodeApi + "/" + swap.InitiatorNodeId
	swapData += `" target="_blank">🔗</a></td></tr>
			<tr><td style="text-align: right">Peer:</td><td style="overflow-wrap: break-word;">`
	swapData += getNodeAlias(swap.PeerNodeId)
	swapData += `&nbsp<a href="`
	swapData += config.Config.NodeApi + "/" + swap.PeerNodeId
	swapData += `" target="_blank">🔗</a></td></tr>
			<tr><td style="text-align: right">Amount:</td><td>`
	swapData += formatWithThousandSeparators(swap.Amount)
	swapData += `</td></tr>
			<tr><td style="text-align: right">ChannelId:</td><td>`
	swapData += swap.ChannelId
	swapData += `</td></tr>`
	if swap.OpeningTxId != "" {
		swapData += `<tr><td style="text-align: right">OpeningTxId:</td><td style="overflow-wrap: break-word;">`
		swapData += swap.OpeningTxId
		swapData += `&nbsp<a href="`
		swapData += url + swap.OpeningTxId
		swapData += `" target="_blank">🔗</a>`
	}
	if swap.ClaimTxId != "" {
		swapData += `</td></tr>
			<tr><td style="text-align: right">ClaimTxId:</td><td style="overflow-wrap: break-word;">`
		swapData += swap.ClaimTxId
		swapData += `&nbsp<a href="`
		swapData += url + swap.ClaimTxId
		swapData += `" target="_blank">🔗</a></td></tr>`
	}
	if swap.CancelMessage != "" {
		swapData += `<tr><td style="text-align: right">CancelMsg:</td><td>`
		swapData += swap.CancelMessage
		swapData += `</td></tr>`
	}
	swapData += `<tr><td style="text-align: right">LndChanId:</td><td>`
	swapData += strconv.FormatUint(uint64(swap.LndChanId), 10)
	swapData += `</td></tr>
		  </table>
		</div>
	  </div>
	</div>
  </div>`

	// Send the updated data as the response
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(swapData))
}

func configHandler(w http.ResponseWriter, r *http.Request) {
	//check for error message to display
	message := ""
	keys, ok := r.URL.Query()["err"]
	if ok && len(keys[0]) > 0 {
		message = keys[0]
	}

	type Page struct {
		Message     string
		ColorScheme string
		Config      config.Configuration
		Version     string
		Latest      string
	}

	data := Page{
		Message:     message,
		ColorScheme: config.Config.ColorScheme,
		Config:      config.Config,
		Version:     version,
		Latest:      getLatestTag(),
	}

	// executing template named "error"
	err := templates.ExecuteTemplate(w, "config", data)
	if err != nil {
		log.Fatalln(err)
		http.Error(w, http.StatusText(500), 500)
	}
}

func liquidHandler(w http.ResponseWriter, r *http.Request) {
	//check for error message to display
	message := ""
	keys, ok := r.URL.Query()["err"]
	if ok && len(keys[0]) > 0 {
		message = keys[0]
	}

	txid := ""
	keys, ok = r.URL.Query()["txid"]
	if ok && len(keys[0]) > 0 {
		txid = keys[0]
	}

	addr := ""
	keys, ok = r.URL.Query()["addr"]
	if ok && len(keys[0]) > 0 {
		addr = keys[0]
	}

	res2, err := ps.LiquidGetBalance()
	if err != nil {
		log.Printf("unable to connect to RPC server: %v", err)
		redirectWithError(w, r, "/?", err)
		return
	}

	var outputs []LiquidUTXO

	if err := listUnspent(&outputs); err != nil {
		log.Printf("unable get listUnspent: %v", err)
		redirectWithError(w, r, "/liquid?", err)
		return
	}

	// sort outputs on Confirmations field
	sort.Slice(outputs, func(i, j int) bool {
		return outputs[i].Confirmations < outputs[j].Confirmations
	})

	type Page struct {
		Message       string
		ColorScheme   string
		LiquidAddress string
		LiquidBalance uint64
		TxId          string
		LiquidUrl     string
		Outputs       *[]LiquidUTXO
		LiquidApi     string
	}

	data := Page{
		Message:       message,
		ColorScheme:   config.Config.ColorScheme,
		LiquidAddress: addr,
		LiquidBalance: res2.GetSatAmount(),
		TxId:          txid,
		LiquidUrl:     config.Config.LiquidApi + "/tx/" + txid,
		Outputs:       &outputs,
		LiquidApi:     config.Config.LiquidApi,
	}

	// executing template named "liquid"
	err = templates.ExecuteTemplate(w, "liquid", data)
	if err != nil {
		log.Fatalln(err)
		http.Error(w, http.StatusText(500), 500)
	}
}

func submitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Parse the form data
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Error parsing form data", http.StatusBadRequest)
			return
		}

		action := r.FormValue("action")
		nodeId := r.FormValue("nodeId")

		switch action {
		case "newAddress":
			res, err := ps.LiquidGetAddress()
			if err != nil {
				log.Printf("unable to connect to RPC server: %v", err)
				redirectWithError(w, r, "/liquid?", err)
				return
			}

			// Redirect to liquid page with new address
			http.Redirect(w, r, "/liquid?msg=\"\"&addr="+res.Address, http.StatusSeeOther)
			return

		case "sendLiquid":
			amt, err := strconv.ParseUint(r.FormValue("sendAmount"), 10, 64)
			if err != nil {
				redirectWithError(w, r, "/liquid?", err)
				return
			}

			txid, err := sendLiquidToAddress(
				r.FormValue("sendAddress"),
				amt,
				r.FormValue("subtractfee") == "on")
			if err != nil {
				redirectWithError(w, r, "/liquid?", err)
				return
			}

			// Redirect to liquid page with TxId
			http.Redirect(w, r, "/liquid?msg=\"\"&txid="+txid, http.StatusSeeOther)
			return
		case "addPeer":
			_, err := ps.AddPeer(nodeId)
			if err != nil {
				redirectWithError(w, r, "/peer?id="+nodeId+"&", err)
				return
			}
			// Redirect to peer page
			http.Redirect(w, r, "/peer?id="+nodeId, http.StatusSeeOther)
			return

		case "removePeer":
			_, err := ps.RemovePeer(nodeId)
			if err != nil {
				redirectWithError(w, r, "/peer?id="+nodeId+"&", err)
				return
			}
			// Redirect to peer page
			http.Redirect(w, r, "/peer?id="+nodeId, http.StatusSeeOther)
			return

		case "suspectPeer":
			_, err := ps.AddSusPeer(nodeId)
			if err != nil {
				redirectWithError(w, r, "/peer?id="+nodeId+"&", err)
				return
			}
			// Redirect to peer page
			http.Redirect(w, r, "/peer?id="+nodeId, http.StatusSeeOther)
			return

		case "unsuspectPeer":
			_, err := ps.RemoveSusPeer(nodeId)
			if err != nil {
				redirectWithError(w, r, "/peer?id="+nodeId+"&", err)
				return
			}
			// Redirect to peer page
			http.Redirect(w, r, "/peer?id="+nodeId, http.StatusSeeOther)
			return

		case "doSwap":
			swapAmount, err := strconv.ParseUint(r.FormValue("swapAmount"), 10, 64)
			if err != nil {
				redirectWithError(w, r, "/peer?id="+nodeId+"&", err)
				return
			}

			channelId, err := strconv.ParseUint(r.FormValue("channelId"), 10, 64)
			if err != nil {
				redirectWithError(w, r, "/peer?id="+nodeId+"&", err)
				return
			}

			switch r.FormValue("direction") {
			case "swapIn":
				resp, err := ps.SwapIn(swapAmount, channelId, r.FormValue("asset"), r.FormValue("force") == "on")
				if err != nil {
					redirectWithError(w, r, "/peer?id="+nodeId+"&", err)
					return
				}
				// Redirect to swap page to follow the swap
				http.Redirect(w, r, "/swap?id="+resp.Swap.Id, http.StatusSeeOther)

			case "swapOut":
				resp, err := ps.SwapOut(swapAmount, channelId, r.FormValue("asset"), r.FormValue("force") == "on")
				if err != nil {
					redirectWithError(w, r, "/peer?id="+nodeId+"&", err)
					return
				}
				// Redirect to swap page to follow the swap
				http.Redirect(w, r, "/swap?id="+resp.Swap.Id, http.StatusSeeOther)
			}

		default:
			// Redirect to index page on any other input
			log.Println("unknonw action: ", action)
			http.Redirect(w, r, "/", http.StatusSeeOther)
		}
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// saves config
func saveConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Parse the form data
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Error parsing form data", http.StatusBadRequest)
			return
		}

		allowSwapRequests, err := strconv.ParseBool(r.FormValue("allowSwapRequests"))
		if err != nil {
			redirectWithError(w, r, "/config?", err)
			return
		}

		config.Config.ColorScheme = r.FormValue("colorScheme")
		config.Config.NodeApi = r.FormValue("nodeApi")
		config.Config.BitcoinApi = r.FormValue("bitcoinApi")
		config.Config.LiquidApi = r.FormValue("liquidApi")

		if config.Config.TelegramToken != r.FormValue("telegramToken") {
			config.Config.TelegramToken = r.FormValue("telegramToken")
			go telegramStart()
		}

		if config.Config.LocalMempool != r.FormValue("localMempool") && r.FormValue("localMempool") != "" {
			// update bitcoinApi link
			config.Config.BitcoinApi = r.FormValue("localMempool")
		}

		config.Config.LocalMempool = r.FormValue("localMempool")

		bitcoinSwaps, err := strconv.ParseBool(r.FormValue("bitcoinSwaps"))
		if err != nil {
			bitcoinSwaps = false
		}

		mustRestart := false
		if config.Config.BitcoinSwaps != bitcoinSwaps || config.Config.ElementsUser != r.FormValue("elementsUser") || config.Config.ElementsPass != r.FormValue("elementsPass") {
			mustRestart = true
		}

		config.Config.BitcoinSwaps = bitcoinSwaps
		config.Config.ElementsUser = r.FormValue("elementsUser")
		config.Config.ElementsPass = r.FormValue("elementsPass")
		config.Config.ElementsDir = r.FormValue("elementsDir")
		config.Config.ElementsDirMapped = r.FormValue("elementsDirMapped")
		config.Config.BitcoinHost = r.FormValue("bitcoinHost")
		config.Config.BitcoinUser = r.FormValue("bitcoinUser")
		config.Config.BitcoinPass = r.FormValue("bitcoinPass")
		config.Config.ProxyURL = r.FormValue("proxyURL")

		mh, err := strconv.ParseUint(r.FormValue("maxHistory"), 10, 16)
		if err != nil {
			redirectWithError(w, r, "/config?", err)
			return
		}
		config.Config.MaxHistory = uint(mh)

		host := r.FormValue("rpcHost")
		clientIsDown := false

		_, err = ps.AllowSwapRequests(host, allowSwapRequests)
		if err != nil {
			// RPC Host entered is bad
			clientIsDown = true
		} else { // values are good, save them
			config.Config.RpcHost = host
			config.Config.AllowSwapRequests = allowSwapRequests
		}

		if err2 := config.Save(); err2 != nil {
			redirectWithError(w, r, "/config?", err2)
			return
		}

		if mustRestart {
			// show progress bar and log
			go http.Redirect(w, r, "/loading", http.StatusSeeOther)
			config.SavePS()
			ps.Stop()
		} else if clientIsDown { // configs did not work, try again
			redirectWithError(w, r, "/config?", err)
		} else { // configs are good
			http.Redirect(w, r, "/", http.StatusSeeOther)
		}
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func stopHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Stopping PeerSwap...", http.StatusBadGateway)
	log.Println("Stop requested")
	go func() {
		ps.Stop()
		os.Exit(0) // Exit the program
	}()
}

func loadingHandler(w http.ResponseWriter, r *http.Request) {
	type Page struct {
		ColorScheme string
		Message     string
		LogPosition int
		LogFile     string
	}

	data := Page{
		ColorScheme: config.Config.ColorScheme,
		Message:     "",
		LogPosition: 0,     // new content and wait for connection
		LogFile:     "log", // peerswapd log
	}

	// executing template named "loading"
	err := templates.ExecuteTemplate(w, "loading", data)
	if err != nil {
		log.Fatalln(err)
		http.Error(w, http.StatusText(500), 500)
	}
}

func backupHandler(w http.ResponseWriter, r *http.Request) {
	wallet := config.Config.ElementsWallet
	// returns .bak with the name of the wallet
	if fileName, err := backupAndZip(wallet); err == nil {
		// Set the Content-Disposition header to suggest a filename
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
		// Serve the file for download
		http.ServeFile(w, r, filepath.Join(config.Config.DataDir, fileName))
		// Delete zip archive
		err = os.Remove(filepath.Join(config.Config.DataDir, fileName))
		if err != nil {
			log.Println("Error deleting zip file:", err)
		}
	} else {
		redirectWithError(w, r, "/liquid?", err)
	}
}

// shows peerswapd log
func logHandler(w http.ResponseWriter, r *http.Request) {
	type Page struct {
		ColorScheme string
		Message     string
		LogPosition int
		LogFile     string
	}

	logFile := "log"

	keys, ok := r.URL.Query()["log"]
	if ok && len(keys[0]) > 0 {
		logFile = keys[0]
	}

	data := Page{
		ColorScheme: config.Config.ColorScheme,
		Message:     "",
		LogPosition: 1, // from first line
		LogFile:     logFile,
	}

	// executing template named "logpage"
	err := templates.ExecuteTemplate(w, "logpage", data)
	if err != nil {
		log.Fatalln(err)
		http.Error(w, http.StatusText(500), 500)
	}
}

// returns log as JSON
func logApiHandler(w http.ResponseWriter, r *http.Request) {

	logText := ""

	keys, ok := r.URL.Query()["pos"]
	if !ok || len(keys[0]) < 1 {
		log.Println("URL parameter 'pos' is missing")
		w.WriteHeader(http.StatusOK)
		return
	}

	startPosition, err := strconv.ParseInt(keys[0], 10, 64)
	if err != nil {
		log.Println("Error:", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	logFile := "log"

	keys, ok = r.URL.Query()["log"]
	if ok && len(keys[0]) > 0 {
		logFile = keys[0]
	}

	filename := filepath.Join(config.Config.DataDir, logFile)

	file, err := os.Open(filename)
	if err != nil {
		log.Println("Error opening file:", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		log.Println("Error getting file info:", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	fileSize := fileInfo.Size()

	if startPosition > 0 && fileSize > startPosition {
		// Seek to the desired starting position
		_, err = file.Seek(startPosition, 0)
		if err != nil {
			log.Println("Error seeking:", err)
			w.WriteHeader(http.StatusOK)
			return
		}

		// Read from the current position till EOF
		content, err := io.ReadAll(file)
		if err != nil {
			log.Println("Error reading file:", err)
			w.WriteHeader(http.StatusOK)
			return
		}

		logText = (string(content))
		length := len(logText)

		if startPosition == 1 && length > 10000 {
			// limit to 10000 characters
			logText = logText[length-10000:]
		}
	}

	// Create a response struct
	type ResponseData struct {
		NextPosition int64
		LogText      string
	}

	responseData := ResponseData{
		NextPosition: fileSize,
		LogText:      logText,
	}

	// Marshal the response struct to JSON
	responseJSON, err := json.Marshal(responseData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send the next chunk of the log as the response
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(responseJSON))
}

func redirectWithError(w http.ResponseWriter, r *http.Request, redirectUrl string, err error) {
	t := fmt.Sprintln(err)
	// translate some errors into plain English
	switch {
	case strings.HasPrefix(t, "rpc error: code = Unavailable desc = connection error"):
		t = "Cannot connect to peerswapd. It either failed to start, awaits LND or has wrong configuration. Check log and peerswap.conf."
	}
	// display the error to the web page header
	msg := url.QueryEscape(t)
	http.Redirect(w, r, redirectUrl+"err="+msg, http.StatusSeeOther)
}

func showHelpMessage() {
	fmt.Println("A lightweight server-side rendered Web UI for PeerSwap LND, which allows trustless P2P submarine swaps Lightning <-> BTC and Lightning <-> L-BTC.")
	fmt.Println("Usage:")
	flag.PrintDefaults()
}

func showVersionInfo() {
	fmt.Println("Version:", version)
}

func startTimer() {
	for range time.Tick(60 * time.Second) {
		// Start Telegram bot if not already running
		go telegramStart()

		// Back up to Telegram if Liquid balance changed
		liquidBackup(false)

		// Check Peg-in status
		if config.Config.PeginTxId != "" {
			confs := ln.GetTxConfirmations(config.Config.PeginTxId)
			if confs >= 102 {
				rawTx, err := getRawTransaction(config.Config.PeginTxId)
				if err == nil {
					proof := getTxOutProof(config.Config.PeginTxId)
					txid, err := claimPegin(rawTx, proof, config.Config.PeginClaimScript)

					// claimpegin takes long time, allow it to timeout
					if err != nil && err.Error() != "timeout reading data from server" {
						log.Println("Peg-in claim FAILED!")
						log.Println("Mainchain TxId:", config.Config.PeginTxId)
						log.Println("Raw tx:", rawTx)
						log.Println("Proof:", proof)
						log.Println("Claim Script:", config.Config.PeginClaimScript)
						telegramSendMessage("❗ Peg-in claim FAILED! See log for details.")
					} else {
						log.Println("Peg-in success! Liquid TxId:", txid)
						telegramSendMessage("💸 Peg-in success!")
					}
				} else {
					log.Println("Peg-In getrawtx FAILED.")
					log.Println("Mainchain TxId:", config.Config.PeginTxId)
					log.Println("Claim Script:", config.Config.PeginClaimScript)
					telegramSendMessage("❗ Peg-In getrawtx FAILED! See log for details.")
				}

				// stop trying after first attempt
				config.Config.PeginTxId = ""
				config.Save()
			}
		}

	}
}

func liquidBackup(force bool) {
	// skip backup if missing RPC or Telegram credentials
	if config.Config.ElementsPass == "" || config.Config.ElementsUser == "" || chatId == 0 {
		return
	}

	res, err := ps.ListActiveSwaps()
	if err != nil {
		return
	}

	// do not backup while a swap is pending
	if len(res.GetSwaps()) > 0 && !force {
		return
	}

	res2, err := ps.LiquidGetBalance()
	if err != nil {
		return
	}

	satAmount := res2.GetSatAmount()

	// do not backup if the sat amount did not change
	if satAmount == config.Config.ElementsBackupAmount && !force {
		return
	}

	wallet := config.Config.ElementsWallet
	destinationZip, err := backupAndZip(wallet)
	if err != nil {
		log.Println("Error zipping backup:", err)
		return
	}

	err = telegramSendFile(config.Config.DataDir, destinationZip, formatWithThousandSeparators(satAmount))
	if err != nil {
		log.Println("Error sending zip:", err)
		return
	}

	// Delete zip archive
	err = os.Remove(filepath.Join(config.Config.DataDir, destinationZip))
	if err != nil {
		log.Println("Error deleting zip file:", err)
	}

	// save the wallet amount
	config.Config.ElementsBackupAmount = satAmount
	config.Save()
}

func bitcoinHandler(w http.ResponseWriter, r *http.Request) {
	//check for error message to display
	message := ""
	keys, ok := r.URL.Query()["err"]
	if ok && len(keys[0]) > 0 {
		message = keys[0]
	}

	var utxos []ln.UTXO
	ln.ListUnspent(&utxos)

	type Page struct {
		Message          string
		ColorScheme      string
		BitcoinBalance   uint64
		Outputs          *[]ln.UTXO
		PeginTxId        string
		PeginAmount      uint64
		BitcoinApi       string
		Confirmations    int32
		Progress         int32
		Duration         string
		SuggestedFeeRate uint32
		MinBumpFeeRate   uint32
	}

	btcBalance := ln.ConfirmedWalletBalance()
	fee := mempool.GetFee()
	confs := int32(0)

	if config.Config.PeginTxId != "" {
		confs = ln.GetTxConfirmations(config.Config.PeginTxId)
		if confs == 0 {
			fee = uint32(float32(fee) * 1.5)
			if fee < config.Config.PeginFeeRate+1 {
				fee = config.Config.PeginFeeRate + 1
			}
		}
	}

	duration := time.Duration(10*(102-confs)) * time.Minute
	formattedDuration := time.Time{}.Add(duration).Format("15h 04m")

	data := Page{
		Message:          message,
		ColorScheme:      config.Config.ColorScheme,
		BitcoinBalance:   uint64(btcBalance),
		Outputs:          &utxos,
		PeginTxId:        config.Config.PeginTxId,
		PeginAmount:      uint64(config.Config.PeginAmount),
		BitcoinApi:       config.Config.BitcoinApi,
		Confirmations:    confs,
		Progress:         int32(confs * 100 / 102),
		Duration:         formattedDuration,
		SuggestedFeeRate: fee,
		MinBumpFeeRate:   config.Config.PeginFeeRate + 1,
	}

	// executing template named "bitcoin"
	err := templates.ExecuteTemplate(w, "bitcoin", data)
	if err != nil {
		log.Fatalln(err)
		http.Error(w, http.StatusText(500), 500)
	}
}

func peginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Parse the form data
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Error parsing form data", http.StatusBadRequest)
			return
		}

		amount, err := strconv.ParseInt(r.FormValue("peginAmount"), 10, 64)
		if err != nil {
			redirectWithError(w, r, "/bitcoin?", err)
			return
		}

		fee, err := strconv.ParseUint(r.FormValue("feeRate"), 10, 64)
		if err != nil {
			redirectWithError(w, r, "/bitcoin?", err)
			return
		}

		btcBalance := ln.ConfirmedWalletBalance()
		sweepall := amount == btcBalance

		// test on pre-existing tx that bitcon core can complete the peg
		tx := "b61ec844027ce18fd3eb91fa7bed8abaa6809c4d3f6cf4952b8ebaa7cd46583a"
		if os.Getenv("NETWORK") == "testnet" {
			tx = "2c7ec5043fe8ee3cb4ce623212c0e52087d3151c9e882a04073cce1688d6fc1e"
		}

		_, err = getRawTransaction(tx)
		if err != nil {
			// automatic fallback to getblock.io
			config.Config.BitcoinHost = config.GetBlockIoHost()
			config.Config.BitcoinUser = ""
			config.Config.BitcoinPass = ""
			_, err = getRawTransaction(tx)
			if err != nil {
				redirectWithError(w, r, "/bitcoin?", errors.New("getrawtransaction request failed, check BitcoinHost in Config"))
				return
			} else {
				// use getblock.io endpoint going forward
				config.Save()
			}
		}

		var addr PeginAddress

		err = getPeginAddress(&addr)
		if err != nil {
			redirectWithError(w, r, "/bitcoin?", err)
			return
		}

		log.Println("Peg-in started to mainchain address:", addr.MainChainAddress, "claim script:", addr.ClaimScript, "amount:", amount)
		duration := time.Duration(1020) * time.Minute
		formattedDuration := time.Time{}.Add(duration).Format("15h 04m")

		telegramSendMessage("⏰ Started peg-in " + formatWithThousandSeparators(uint64(amount)) + " sats. Time left: " + formattedDuration)

		config.Config.PeginClaimScript = addr.ClaimScript
		config.Config.PeginAmount = amount
		config.Save()

		txid, err := ln.SendCoins(addr.MainChainAddress, amount, fee, sweepall, "Liquid pegin")
		if err != nil {
			redirectWithError(w, r, "/bitcoin?", err)
			return
		}

		config.Config.PeginTxId = txid
		config.Config.PeginFeeRate = uint32(fee)
		config.Save()

		// Redirect to bitcoin page to follow the pegin progress
		http.Redirect(w, r, "/bitcoin", http.StatusSeeOther)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func bumpfeeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Parse the form data
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Error parsing form data", http.StatusBadRequest)
			return
		}

		fee, err := strconv.ParseUint(r.FormValue("feeRate"), 10, 64)
		if err != nil {
			redirectWithError(w, r, "/bitcoin?", err)
			return
		}

		if config.Config.PeginTxId == "" {
			redirectWithError(w, r, "/bitcoin?", errors.New("no pending peg-in"))
			return
		}

		tx, err := ln.GetTransaction(config.Config.PeginTxId)
		if err != nil {
			redirectWithError(w, r, "/bitcoin?", err)
			return
		}

		index := uint32(0)
		for i, output := range tx.OutputDetails {
			if output.Amount != config.Config.PeginAmount {
				index = uint32(i)
				break
			}
		}

		if tx.OutputDetails[index].Amount == config.Config.PeginAmount {
			redirectWithError(w, r, "/bitcoin?", errors.New("peg-in tx has no change, not possible to bump"))
			return
		}

		err = ln.BumpFee(config.Config.PeginTxId, uint32(index), fee)
		if err != nil {
			redirectWithError(w, r, "/bitcoin?", err)
			return
		}

		// save the new rate, so the next bump cannot be lower
		config.Config.PeginFeeRate = uint32(fee)
		config.Save()

		// Redirect to bitcoin page to follow the pegin progress
		http.Redirect(w, r, "/bitcoin", http.StatusSeeOther)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func setLogging() error {
	// Set log file name
	logFileName := filepath.Join(config.Config.DataDir, "psweb.log")
	var err error
	// Open log file in append mode, create if it doesn't exist
	logFile, err = os.OpenFile(logFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	// Set log output to both file and standard output
	multi := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(multi)

	log.SetFlags(log.Ldate | log.Ltime)
	if os.Getenv("DEBUG") == "1" {
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	}

	return nil
}

func closeLogFile() {
	if logFile != nil {
		if err := logFile.Close(); err != nil {
			log.Println("Error closing log file:", err)
		}
	}
}

func findPeerById(peers []*peerswaprpc.PeerSwapPeer, targetId string) *peerswaprpc.PeerSwapPeer {
	for _, p := range peers {
		if p.NodeId == targetId {
			return p
		}
	}
	return nil // Return nil if peer with given ID is not found
}

// converts a list of peers into an HTML table to display
func convertPeersToHTMLTable(peers []*peerswaprpc.PeerSwapPeer, allowlistedPeers []string, suspiciousPeers []string) string {

	type Table struct {
		AvgLocal uint64
		HtmlBlob string
	}

	var unsortedTable []Table

	for _, peer := range peers {
		var totalLocal uint64
		var totalCapacity uint64

		table := "<table style=\"table-layout:fixed; width: 100%\">"
		table += "<tr style=\"border: 1px dotted\">"
		table += "<td id=\"scramble\" style=\"float: left; text-align: left; width: 70%;\">"

		// alias is a link to open peer details page
		table += "<a href=\"/peer?id=" + peer.NodeId + "\">"

		if stringIsInSlice(peer.NodeId, allowlistedPeers) {
			table += "✅&nbsp"
		} else {
			table += "⛔&nbsp"
		}

		if stringIsInSlice(peer.NodeId, suspiciousPeers) {
			table += "🔍&nbsp"
		}

		table += getNodeAlias(peer.NodeId)
		table += "</a>"
		table += "</td><td style=\"float: right; text-align: right; width:30%;\">"
		table += "<a href=\"/peer?id=" + peer.NodeId + "\">"

		if stringIsInSlice("lbtc", peer.SupportedAssets) {
			table += "🌊&nbsp"
		}
		if stringIsInSlice("btc", peer.SupportedAssets) {
			table += "₿&nbsp"
		}
		if peer.SwapsAllowed {
			table += "✅"
		} else {
			table += "⛔"
		}
		table += "</a>"
		table += "</td></tr></table>"

		table += "<table style=\"table-layout:fixed;\">"

		// Construct channels data
		for _, channel := range peer.Channels {

			// red background for inactive channels
			bc := "#590202"
			if config.Config.ColorScheme == "light" {
				bc = "#fcb6b6"
			}

			if channel.Active {
				// green background for active channels
				bc = "#224725"
				if config.Config.ColorScheme == "light" {
					bc = "#e6ffe8"
				}
			}

			table += "<tr style=\"background-color: " + bc + "\"; >"
			table += "<td id=\"scramble\" style=\"width: 20ch; text-align: center\">"
			table += formatWithThousandSeparators(channel.LocalBalance)
			table += "</td><td style=\"width: 50%; text-align: center\">"
			local := channel.LocalBalance
			capacity := channel.LocalBalance + channel.RemoteBalance
			totalLocal += local
			totalCapacity += capacity
			table += "<a href=\"/peer?id=" + peer.NodeId + "\">"
			table += "<progress style=\"width: 100%;\" value=" + strconv.FormatUint(local, 10) + " max=" + strconv.FormatUint(capacity, 10) + ">1</progress>"
			table += "</a></td>"
			table += "<td id=\"scramble\" style=\"width: 20ch; text-align: center\">"
			table += formatWithThousandSeparators(channel.RemoteBalance)
			table += "</td></tr>"
		}
		table += "</table>"
		table += "<p style=\"margin:0.5em;\"></p>"

		// count total outbound to sort peers later
		pct := uint64(1000000 * float64(totalLocal) / float64(totalCapacity))

		unsortedTable = append(unsortedTable, Table{
			AvgLocal: pct,
			HtmlBlob: table,
		})
	}

	// sort the table on AvgLocal field
	sort.Slice(unsortedTable, func(i, j int) bool {
		return unsortedTable[i].AvgLocal < unsortedTable[j].AvgLocal
	})

	table := ""
	for _, t := range unsortedTable {
		table += t.HtmlBlob
	}

	return table
}

// converts a list of swaps into an HTML table
func convertSwapsToHTMLTable(swaps []*peerswaprpc.PrettyPrintSwap) string {

	if len(swaps) == 0 {
		return ""
	}

	type Table struct {
		TimeStamp int64
		HtmlBlob  string
	}
	var unsortedTable []Table

	for _, swap := range swaps {
		table := "<tr>"
		table += "<td style=\"width: 25%; text-align: left\">"

		tm := timePassedAgo(time.Unix(swap.CreatedAt, 0).UTC())

		// clicking on timestamp will open swap details page
		table += "<a href=\"/swap?id=" + swap.Id + "\">" + tm + "</a> "
		table += "</td><td style=\"text-align: left\">"
		table += visualiseSwapStatus(swap.State, false) + "&nbsp"
		table += formatWithThousandSeparators(swap.Amount)

		asset := "🌊"
		if swap.Asset == "btc" {
			asset = "<span style=\"color: orange;\">₿</span>"
		}

		switch swap.Type + swap.Role {
		case "swap-outsender":
			table += " ⚡&nbsp⇨&nbsp" + asset
		case "swap-insender":
			table += " " + asset + "&nbsp⇨&nbsp⚡"
		case "swap-outreceiver":
			table += " " + asset + "&nbsp⇨&nbsp⚡"
		case "swap-inreceiver":
			table += " ⚡&nbsp⇨&nbsp" + asset
		}

		table += "</td><td id=\"scramble\" style=\"overflow-wrap: break-word;\">"

		switch swap.Role {
		case "receiver":
			table += " ⇦&nbsp"
		case "sender":
			table += " ⇨&nbsp"
		default:
			table += " ?&nbsp"
		}

		table += getNodeAlias(swap.PeerNodeId)
		table += "</td></tr>"

		unsortedTable = append(unsortedTable, Table{
			TimeStamp: swap.CreatedAt,
			HtmlBlob:  table,
		})
	}

	// sort the table on TimeStamp field
	sort.Slice(unsortedTable, func(i, j int) bool {
		return unsortedTable[i].TimeStamp > unsortedTable[j].TimeStamp
	})

	var counter uint
	table := "<table style=\"table-layout:fixed; width: 100%\">"
	for _, t := range unsortedTable {
		counter++
		if counter > config.Config.MaxHistory {
			break
		}
		table += t.HtmlBlob
	}
	table += "</table>"
	return table
}
