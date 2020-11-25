package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Showmax/go-fqdn"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/net/publicsuffix"

	"isc.org/stork/pki"
)

// Paths pointing to agent's key and cert, and CA cert from server,
// and agent token generated by agent.
// They are being modified by tests so need to be writable.
var (
	KeyPEMFile     = "/var/lib/stork-agent/certs/key.pem"          // nolint:gochecknoglobals
	CertPEMFile    = "/var/lib/stork-agent/certs/cert.pem"         // nolint:gochecknoglobals
	RootCAFile     = "/var/lib/stork-agent/certs/ca.pem"           // nolint:gochecknoglobals
	AgentTokenFile = "/var/lib/stork-agent/tokens/agent-token.txt" // nolint:gochecknoglobals,gosec
)

// Create HTTP client with persistent cookies in memory.
// It is used to communicate with Stork Server.
func newSrvClient() (*http.Client, bool) {
	options := cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	}
	jar, err := cookiejar.New(&options)
	if err != nil {
		log.Errorf("problem with cookiejar: %s", err)
		return nil, false
	}
	client := &http.Client{
		Jar: jar,
	}
	return client, true
}

// Prompt user for server token.
func getServerTokenFromUser() (string, error) {
	fmt.Printf(">>>> Please, provide server access token (optional): ")
	serverToken, err := terminal.ReadPassword(0)
	fmt.Print("\n")
	if err != nil {
		return "", err
	}
	return string(serverToken), nil
}

// Get agent's address and port from user if not provided via command line options.
func getAgentAddrAndPortFromUser(agentAddr, agentPort string) (string, int, error) {
	if agentAddr == "" {
		agentAddrTip, err := fqdn.FqdnHostname()
		msg := ">>>> Please, provide address (IP or name/FQDN) of current host with Stork Agent (it will be used to connect from Stork Server)"
		if err != nil {
			agentAddrTip = ""
			msg += ": "
		} else {
			msg += fmt.Sprintf(" [%s]: ", agentAddrTip)
		}
		fmt.Println(msg)
		fmt.Scanln(&agentAddr)
		if agentAddr == "" {
			agentAddr = agentAddrTip
		}
	}

	if agentPort == "" {
		fmt.Printf(">>>> Please, provide port that Stork Agent will use to listen on [8080]: ")
		fmt.Scanln(&agentPort)
		if agentPort == "" {
			agentPort = "8080"
		}
	}

	agentPortInt, err := strconv.Atoi(agentPort)
	if err != nil {
		log.Errorf("cannot parse agent port: %s: %s", agentPort, err)
		return "", 0, err
	}
	return agentAddr, agentPortInt, nil
}

// Write agent file. Used to save key or certs.
// They are sensitive so perms are set to 0600.
func writeAgentFile(path string, content []byte) error {
	_, err := os.Stat(path)
	if os.IsExist(err) {
		err = os.Remove(path)
		if err != nil {
			return err
		}
	}

	err = ioutil.WriteFile(path, content, 0600)
	if err != nil {
		return err
	}
	return nil
}

// Parse provided address and return either IP or name.
func resolveAddr(addr string) ([]net.IP, []string) {
	var agntIPs []net.IP
	var agntNames []string

	ipAddr := net.ParseIP(addr)
	if ipAddr == nil {
		agntNames = append(agntNames, addr)
	} else {
		agntIPs = append(agntIPs, ipAddr)
	}

	return agntIPs, agntNames
}

// Generate or regenerate agent key and CSR.
func generateCerts(agentAddr string, regenCerts bool) ([]byte, string, error) {
	regenCerts2 := regenCerts
	_, err := os.Stat(KeyPEMFile)
	if !os.IsExist(err) {
		regenCerts2 = true
	}

	agntIPs, agntNames := resolveAddr(agentAddr)

	var fingerprint [32]byte
	var csrPEM []byte
	var privKeyPEM []byte
	if regenCerts2 {
		// generate private key and CSR
		privKeyPEM, csrPEM, fingerprint, err = pki.GenKeyAndCSR("agent", agntNames, agntIPs)
		if err != nil {
			return nil, "", err
		}

		// save private key to file
		err = writeAgentFile(KeyPEMFile, privKeyPEM)
		if err != nil {
			return nil, "", err
		}
		log.Printf("agent key and CSR (re)generated")
	} else {
		// generate CSR using existing private key
		privKeyPEM, err = ioutil.ReadFile(KeyPEMFile)
		if err != nil {
			return nil, "", errors.Wrapf(err, "could not load key PEM file: %s", KeyPEMFile)
		}

		csrPEM, fingerprint, err = pki.GenCSRUsingKey("agent", agntNames, agntIPs, privKeyPEM)
		if err != nil {
			return nil, "", err
		}
		log.Printf("loaded existing agent key and generated CSR")
	}

	// convert fingerpring to hex string
	var buf bytes.Buffer
	for _, f := range fingerprint {
		fmt.Fprintf(&buf, "%02X", f)
	}
	fingerprintStr := buf.String()

	return csrPEM, fingerprintStr, nil
}

// Prepare agent registration request to Stork server in JSON format.
func prepareRegistrationRequest(csrPEM []byte, serverToken, agentToken, agentAddr string, agentPort int) (*bytes.Buffer, bool) {
	values := map[string]interface{}{
		"address":     agentAddr,
		"agentPort":   agentPort,
		"agentCSR":    string(csrPEM),
		"serverToken": serverToken,
		"agentToken":  agentToken,
	}
	jsonValue, err := json.Marshal(values)
	if err != nil {
		log.Errorf("cannot marshal registration request: %s", err)
		return nil, false
	}
	return bytes.NewBuffer(jsonValue), true
}

// Register agent in Stork server under provided URL using body in request.
// If retry is true then registration is repeated until it connection to server
// is established. This case is used when agent automatically tries to register
// during startup.
func srvRegister(client *http.Client, baseSrvURL *url.URL, body *bytes.Buffer, retry bool) (int, string, string, bool) {
	url, _ := baseSrvURL.Parse("api/machines")
	var err error
	var resp *http.Response
	for {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url.String(), body)
		if err != nil {
			log.Errorf("problem with preparing registering request: %s", err)
			return 0, "", "", false
		}
		req.Header.Add("Content-Type", "application/json")
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		if retry && strings.Contains(err.Error(), "connection refused") {
			log.Println("sleeping for 10 seconds before next registration attempt")
			time.Sleep(10 * time.Second)
		} else {
			log.Errorf("problem with registering machine: %s", err)
			return 0, "", "", false
		}
	}
	data, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Errorf("problem with registering machine: %s", err)
		return 0, "", "", false
	}
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		log.Errorf("problem with parsing response from registering machine: %s", err)
		return 0, "", "", false
	}
	errTxt := result["error"]
	if errTxt != nil {
		log.Errorf("problem with registering machine: %s", errTxt)
		return 0, "", "", false
	}
	if resp.StatusCode >= 400 {
		errTxt = result["message"]
		if errTxt != nil {
			log.Errorf("problem with registering machine: %s", errTxt)
		} else {
			log.Errorf("problem with registering machine: http status code %d", resp.StatusCode)
		}
		return 0, "", "", false
	}
	log.Printf("machine registered")
	return int(result["id"].(float64)), result["serverCACert"].(string), result["agentCert"].(string), true
}

// Check certs received from server.
func checkAndStoreCerts(serverCACert, agentCert string) error {
	// check certs
	_, err := pki.ParseCert([]byte(serverCACert))
	if err != nil {
		return errors.Wrapf(err, "cannot parse server CA cert")
	}
	_, err = pki.ParseCert([]byte(agentCert))
	if err != nil {
		return errors.Wrapf(err, "cannot parse agent cert")
	}

	// save certs
	err = writeAgentFile(CertPEMFile, []byte(agentCert))
	if err != nil {
		return errors.Wrapf(err, "cannot write agent cert")
	}
	err = writeAgentFile(RootCAFile, []byte(serverCACert))
	if err != nil {
		return errors.Wrapf(err, "cannot write server CA cert")
	}
	log.Printf("stored agent signed cert and CA cert")
	return nil
}

// Ping Stork agent service via Stork server. It is used during manual registration
// to confirm that TLS connection between agent and server can be established.
func srvPing(client *http.Client, baseSrvURL *url.URL, machineID int, serverToken, agentToken string) bool {
	urlSuffix := fmt.Sprintf("api/machines/%d/ping", machineID)
	url, err := baseSrvURL.Parse(urlSuffix)
	if err != nil {
		log.Errorf("problem with preparing url %s + %s: %s", baseSrvURL.String(), urlSuffix, err)
		return false
	}
	req := map[string]interface{}{
		"serverToken": serverToken,
		"agentToken":  agentToken,
	}
	jsonReq, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url.String(), bytes.NewBuffer(jsonReq))
	if err != nil {
		log.Errorf("problem with preparing http request: %s", err)
		return false
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Errorf("problem with pinging machine: %s", err)
		return false
	}
	data, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Errorf("problem with pinging machine: %s", err)
		return false
	}
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		log.Errorf("problem with parsing response from pinging machine: %s", err)
		return false
	}
	errTxt := result["error"]
	if errTxt != nil {
		log.Errorf("problem with pinging machine: %s", errTxt)
		return false
	}
	if resp.StatusCode >= 400 {
		errTxt = result["message"]
		if errTxt != nil {
			log.Warnf("problem with pinging machine: %s", errTxt)
		} else {
			log.Warnf("problem with pinging machine: http status code %d", resp.StatusCode)
		}
		return false
	}

	log.Printf("machine ping over TLS: OK")

	return true
}

// Main function used to register an agent with given address and port in given server by URL.
// If regenCerts is true then agent key and cert is regenerated, otherwise the ones stored in files
// are used. RegenCerts is used when registration is run manually. If retry is true then registration
// is retried if connection to server cannot be established. This case is used when registration
// is automatic during agent service startup. Server token can be provided in manual registration
// via command line switch. This way the agent will be immediately authorized in the server.
// If server token is empty (in automatic registration or when it is not provided
// in manual registration) then agent is added to server but requires manual authorization in web UI.
func Register(serverURL, serverToken, agentAddr, agentPort string, regenCerts bool, retry bool) bool {
	// parse URL to server
	baseSrvURL, err := url.Parse(serverURL)
	if err != nil {
		log.Errorf("cannot parse server URL: %s: %s", serverURL, err)
		return false
	}

	// prepare http client to connect to Stork server
	client, ok := newSrvClient()
	if !ok {
		return false
	}

	// Get server token from user (if not provided in cmd line) to authenticate in the server.
	// Do not ask if regenCerts is true (ie. Register is called from agent).
	serverToken2 := serverToken
	if serverToken == "" && regenCerts {
		serverToken2, err = getServerTokenFromUser()
		if err != nil {
			log.Errorf("problem with getting password: %s", err)
			return false
		}
	}

	agentAddr, agentPortInt, err := getAgentAddrAndPortFromUser(agentAddr, agentPort)
	if err != nil {
		return false
	}

	// Generate agent private key and cert. If they already exist then regenerate them if forced.
	csrPEM, fingerprint, err := generateCerts(agentAddr, regenCerts)
	if err != nil {
		log.Errorf("problem with generating certs: %s", err)
		return false
	}

	// If server token was not provided then use cert fingerprint as agent token.
	// Agent token is another mode for checking identity of an agent.
	var agentToken string
	if serverToken2 == "" {
		agentToken = fingerprint
		log.Println("=============================================================================")
		log.Printf("AGENT TOKEN: %s", fingerprint)
		log.Println("=============================================================================")
		err = writeAgentFile(AgentTokenFile, []byte(fingerprint))
		if err != nil {
			log.Errorf("problem with storing agent token to %s: %s", AgentTokenFile, err)
			return false
		}
		log.Printf("agent token stored to %s", AgentTokenFile)
		log.Printf("authorize machine in Stork web UI")
	} else {
		log.Printf("machine will be automatically authorized using server token")
	}

	// register new machine i.e. current agent
	req, ok := prepareRegistrationRequest(csrPEM, serverToken2, agentToken, agentAddr, agentPortInt)
	if !ok {
		return false
	}
	log.Println("try to register agent in Stork server")
	machineID, serverCACert, agentCert, ok := srvRegister(client, baseSrvURL, req, retry)
	if !ok {
		return false
	}

	// store certs
	err = checkAndStoreCerts(serverCACert, agentCert)
	if err != nil {
		log.Errorf("problem with certs: %s", err)
		return false
	}

	if serverToken2 != "" {
		// invoke getting machine state via server
		ok = false
		for i := time.Duration(1); i < 4; i++ {
			ok = srvPing(client, baseSrvURL, machineID, serverToken2, agentToken)
			if ok {
				break
			}
			if i < 3 {
				log.Printf("retrying ping")
				time.Sleep(2 * i * time.Second)
			}
		}
		if !ok {
			log.Errorf("cannot ping machine")
			return false
		}
	}

	return true
}
