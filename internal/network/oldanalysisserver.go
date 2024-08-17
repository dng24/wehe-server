// Handles analysis server connections from Wehe clients < v4.0
package network

import (
    "crypto/tls"
    "fmt"
    "net/http"
    "net/url"
    "strconv"
    "sync"

    "wehe-server/internal/clienthandler"
)

const (
    analyzerHTTPSPort = 56566
)

var (
    // TODO: if old client does not make get results http request, clients will be stuck in here forever
    unanalyzedTests = &analysisServerClient{
        clients: make(map[string]*clienthandler.Client),
    }
)

// The old server uses different side channels and server for each replay and analysis. This
// requires state to be kept between each replay and analysis for each test. This struct is used to
// keep that state. 
type analysisServerClient struct {
    // contains all the client information; key is the userID + testID
    clients map[string]*clienthandler.Client
    mutex sync.Mutex
}

func (asc *analysisServerClient) addClient(client *clienthandler.Client) {
    asc.mutex.Lock()
    defer asc.mutex.Unlock()
    key := client.UserID + strconv.Itoa(client.TestID)
    asc.clients[key] = client
}

func (asc *analysisServerClient) getClient(userID string, testID string) (*clienthandler.Client, bool) {
    asc.mutex.Lock()
    defer asc.mutex.Unlock()
    client, exists := asc.clients[userID + testID]
    return client, exists
}

func (asc *analysisServerClient) deleteClient(userID string, testID string) {
    asc.mutex.Lock()
    defer asc.mutex.Unlock()
    delete(asc.clients, userID + testID)
}

// Starts the old HTTPS analyzer server.
// cert: TLS cert to be used for the server
// errChan: error channel to return errors
func StartOldAnalyzerServer(cert tls.Certificate, errChan chan<- error) {
    http.HandleFunc("/Results", oldHandleRequest)

    fmt.Println("Listening on old analysis server", analyzerHTTPSPort)
    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{cert},
    }
    server := &http.Server{
        Addr: fmt.Sprintf(":%d", analyzerHTTPSPort),
        TLSConfig: tlsConfig,
    }
    err := server.ListenAndServeTLS("", "")
    errChan <- err
}

// The old analysis server mainly contains two functions:
// 1. a POST request, which tells the server to analyze a test
// 2. a GET request, which retrieves the analysis result from the server
// w: HTTP output channel
// r: the HTTP request
func oldHandleRequest(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost {
        oldAnalyzeTest(w, r)
    } else if r.Method == http.MethodGet {
        oldGetResult(w, r)
    } else {
        http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
    }
}

// Receives the POST request to analyze a test. In this new implementation, analysis is actually
// called in oldsidechannel.go:handleOldSideChannel after throughputs are received. The analysis
// takes too long, as the old client keep timing out before the analysis is ready to be returned.
// Calling the analysis in the side channel allows the client to be blocked until the analysis
// results are ready.
// w: HTTP output channel
// r: the HTTP request
func oldAnalyzeTest(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("{\"success\": true}"))
}

// Gets a query parameters value from a GET request.
// w: HTTP output channel
// queryParams: the query parameters of the GET request
// key: the query parameter to get
// Returns the value of the parameter and if the key exists. If the key does not exist, an error
// is sent to the client.
func getFormValue(w http.ResponseWriter, queryParams url.Values, key string) (string, bool) {
    value := queryParams.Get(key)
    if value == "" {
        w.Write([]byte(fmt.Sprintf("{\"success\": false, \"error\": \"%s not provided\"}", key)))
        return "", false
    }
    return value, true
}

// Receives the GET request to retrieve the analysis result.
// w: HTTP output channel
// r: the HTTP request
func oldGetResult(w http.ResponseWriter, r *http.Request) {
    fmt.Println("GET path:", r.URL.Path, "GET query:", r.URL.RawQuery)

    w.WriteHeader(http.StatusOK)

    // parse the parameters
    queryParams := r.URL.Query()

    command, success := getFormValue(w, queryParams, "command")
    if !success {
        return
    }
    if command != "singleResult" {
        w.Write([]byte("{\"success\": false, \"error\": \"unknown command\"}"))
        return
    }

    userID, success := getFormValue(w, queryParams, "userID")
    if !success {
        return
    }

    historyCountStr, success := getFormValue(w, queryParams, "historyCount")
    if !success {
        return
    }

    testIDStr, success := getFormValue(w, queryParams, "testID")
    if !success {
        return
    }

    testID, err := strconv.Atoi(testIDStr)
    if err != nil {
        fmt.Println(err)
        w.Write([]byte("{\"success\": false, \"error\": \"%v\"}"))
        return
    }

    // Gets the client object that contains the results
    clt, exists := unanalyzedTests.getClient(userID, historyCountStr)
    if !exists {
        w.Write([]byte("{\"success\": false, \"error\": \"No result found\"}"))
        return
    }

    var replay clienthandler.ReplayResult
    found := false
    // The old client makes a result request for a specific replay (later versions of the old
    // server will make a request for the random replay). Note that "testID" in this section is
    // equivalent to the replay ID in the new server. Test ID in the new server is equivalent to
    // historyCount in the old server.
    for _, r := range clt.ReplayResults {
        if testID == int(r.ReplayID) {
            replay = r
            found = true
            break
        }
    }

    if !found {
        w.Write([]byte("{\"success\": false, \"error\": \"No result found\"}"))
        return
    }

    if clt.Analysis == nil {
        w.Write([]byte("{\"success\": false, \"error\": \"No result found\"}"))
        return
    }

    dateFormatted := clt.StartTime.Format("2006-01-02 15:04:05")

    result := fmt.Sprintf("{\"success\": true, \"response\": {\"replayName\": \"%s\", \"date\": \"%s\", \"userID\": \"%s\", \"extraString\": \"%s\", \"historyCount\": \"%s\", \"testID\": \"%s\", \"area_test\": \"%f\", \"ks2_ratio_test\": \"%f\", \"xput_avg_original\": \"%f\", \"xput_avg_test\": \"%f\", \"ks2dVal\": \"%f\", \"ks2pVal\": \"%f\"}}", replay.ReplayName, dateFormatted, userID, clt.ExtraString, historyCountStr, testIDStr, clt.Analysis.Area0var, clt.Analysis.KS2AcceptRatio, clt.Analysis.OriginalReplayStats.Average, clt.Analysis.RandomReplayStats.Average, clt.Analysis.KS2dVal, clt.Analysis.KS2pVal)
    fmt.Println("sending:", result)
    w.Write([]byte(result))

    unanalyzedTests.deleteClient(userID, historyCountStr)
}
