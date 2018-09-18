package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/antongulenko/go-bitflow-pipeline/bitflow-script/reg"
	"github.com/shirou/gopsutil/cpu"
	log "github.com/sirupsen/logrus"
)

const (
	StatusCreated        = "created"
	StatusRunning        = "running"
	StatusFinished       = "finished"
	StatusFailed         = "failed"
	StatusKilled         = "killing"
	StatusKilledFinished = "killed"
)

type SubprocessEngine struct {
	Executable string
	Tags       map[string]string

	capabilities  reg.ProcessingSteps
	pipelines     map[int]*RunningPipeline
	pipelinesLock sync.Mutex

	nextId     int
	nextIdLock sync.Mutex

	lastCpuTimes    []cpu.TimesStat
	currentCpuUsage []float64
}

type RunningPipeline struct {
	Id          int
	Script      string
	ExtraParams []string
	Status      string
	Errors      string

	// Not exported in JSON
	engine *SubprocessEngine
	cmd    *exec.Cmd
	output bytes.Buffer
	lock   sync.Mutex
}

func (engine *SubprocessEngine) Run() (err error) {
	engine.capabilities, err = LoadCapabilities(engine.Executable)
	if err != nil {
		return
	}
	engine.pipelines = make(map[int]*RunningPipeline)
	go engine.ObserveCpuTimes()
	return nil
}

func (engine *SubprocessEngine) NewPipeline(script string, delay time.Duration, extraParams []string) (*RunningPipeline, error) {
	pipe := &RunningPipeline{
		Id:          engine.getNextId(),
		Script:      script,
		ExtraParams: extraParams,
		Status:      StatusCreated,
		engine:      engine,
	}
	engine.pipelinesLock.Lock()
	engine.pipelines[pipe.Id] = pipe
	engine.pipelinesLock.Unlock()
	err := pipe.Run()
	if delay > 0 && err == nil {
		time.Sleep(delay)
		if pipe.Status == StatusFailed {
			err = fmt.Errorf("The pipeline %v failed within %v with the following output:\n%v",
				pipe.Id, delay, pipe.output.String())
		}
	}
	return pipe, err
}

func (engine *SubprocessEngine) getNextId() int {
	engine.nextIdLock.Lock()
	defer engine.nextIdLock.Unlock()
	result := engine.nextId
	engine.nextId++
	return result
}

func (pipe *RunningPipeline) Run() error {
	pipe.lock.Lock()
	defer pipe.lock.Unlock()

	if pipe.Status != StatusCreated {
		return fmt.Errorf("The pipeline %v has already been started", pipe.Id)
	}

	// The space in front of the script is to avoid the script to be confused with a flag that starts with -
	pipe.cmd = exec.Command(pipe.engine.Executable, append(pipe.ExtraParams, " "+pipe.Script)...)
	pipe.cmd.Stderr = &pipe.output
	pipe.cmd.Stdout = &pipe.output
	err := pipe.cmd.Start()
	if err != nil {
		pipe.addError(StatusFailed, err)
	} else {
		go pipe.waitForProcess()
		pipe.setStatus(StatusRunning)
	}
	return err
}

func (pipe *RunningPipeline) waitForProcess() {
	err := pipe.cmd.Wait()
	pipe.lock.Lock()
	defer pipe.lock.Unlock()

	if pipe.Status == StatusKilled {
		pipe.addError(StatusKilledFinished, err)
	} else {
		if err == nil {
			pipe.setStatus(StatusFinished)
		} else {
			pipe.addError(StatusFailed, err)
		}
	}
}

func (pipe *RunningPipeline) Kill() error {
	pipe.lock.Lock()
	defer pipe.lock.Unlock()

	if pipe.Status != StatusRunning {
		return fmt.Errorf("The pipeline %v is not running (%v)", pipe.Id, pipe.Status)
	}
	err := pipe.cmd.Process.Kill()
	if err != nil {
		// TODO in case of kill-error, put the entire pipeline in error state?
		// TODO Take other measures to clean up the pipeline?
		pipe.addError("", err)
		return fmt.Errorf("Error killing process of pipeline %v: %v", pipe.Id, err)
	}
	pipe.setStatus(StatusKilled)
	return nil
}

func (pipe *RunningPipeline) GetOutput() ([]byte, error) {
	pipe.lock.Lock()
	defer pipe.lock.Unlock()
	if pipe.Status == StatusCreated {
		return nil, fmt.Errorf("The pipeline %v has not been started yet", pipe.Id)
	}
	return pipe.output.Bytes(), nil
}

func (pipe *RunningPipeline) setStatus(status string) {
	pipe.addError(status, nil)
}

func (pipe *RunningPipeline) addError(status string, err error) {
	if status != "" {
		if pipe.Status != StatusCreated && pipe.Status != StatusRunning {
			// Warn if the status was already final
			log.Warnf("Pipeline %v status changed from %v to %v", pipe.Id, pipe.Status, status)
		}
		pipe.Status = status
	}
	if err != nil {
		if pipe.Errors == "" {
			pipe.Errors = err.Error()
		} else {
			pipe.Errors += "\n" + err.Error()
		}
	}
}
