package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"

	"io/ioutil"
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/antongulenko/go-bitflow-pipeline/query"
	"github.com/antongulenko/golib"
)

const (
	defaultExecutableName      = "bitflow-pipeline"
	managerNotificationDelay   = 200 * time.Millisecond
	managerNotificationTimeout = 2000 * time.Millisecond
)

func main() {
	var executable, httpEndpoint, managerURL string
	flag.StringVar(&executable, "e", "", fmt.Sprintf("Name of the pipeline executable. By default, search $PATH for %v", defaultExecutableName))
	flag.StringVar(&httpEndpoint, "h", ":8080", "HTTP endpoint for serving the REST API.")
	flag.StringVar(&managerURL, "m", "", "After initializing the REST API, send a GET request with no further headers or content to the given URL.")
	flag.Parse()

	var err error
	if executable == "" {
		executable, err = exec.LookPath(defaultExecutableName)
		golib.Checkerr(err)
		log.Println("Using executable:", executable)
	}

	engine := SubprocessEngine{
		Executable: executable,
	}
	golib.Checkerr(engine.Run())
	if managerURL != "" {
		notifyManager(managerURL)
	}
	golib.Checkerr(engine.ServeHttp(httpEndpoint))
}

const CapabilitiesFlag = "-capabilities"

func LoadCapabilities(executable string) (obj query.ProcessingSteps, err error) {
	var output []byte
	cmd := exec.Command(executable, CapabilitiesFlag)
	cmd.Stderr = os.Stderr
	output, err = cmd.Output()
	if err == nil {
		err = json.Unmarshal(output, &obj)
	}
	if err != nil {
		err = fmt.Errorf("Error obtaining capabilities of %v: %v", executable, err)
	}
	return
}

func notifyManager(managerURL string) {
	// Defer the management registration shortly, to first initialize the HTTP server below.
	// Unfortunately there is no cleaner way to do this.
	time.AfterFunc(200*time.Millisecond, func() {
		log.Printf("Notifying the manager at %v...", managerURL)

		client := http.Client{
			Timeout: managerNotificationTimeout,
		}
		resp, err := client.Get(managerURL)
		if err == nil && resp.StatusCode != http.StatusOK {
			body, _ := ioutil.ReadAll(resp.Body) // Ignore the read error
			err = fmt.Errorf("Received non-success response (status %v %v) from manager (URL: %v). Body:\n%v",
				resp.StatusCode, resp.Status, managerURL, string(body))
		}
		if err != nil {
			log.Fatalln(err)
		}
	})
}
