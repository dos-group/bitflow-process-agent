package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"

	log "github.com/Sirupsen/logrus"
	"github.com/antongulenko/go-bitflow-pipeline/query"
	"github.com/antongulenko/golib"
)

const defaultExecutableName = "bitflow-pipeline"

func main() {
	var executable, httpEndpoint string
	flag.StringVar(&executable, "e", "", fmt.Sprintf("Name of the pipeline executable. By default, search PATH for %v", defaultExecutableName))
	flag.StringVar(&httpEndpoint, "h", ":8080", "HTTP endpoint for serving REST API")
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
