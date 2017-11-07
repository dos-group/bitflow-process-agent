package main

import (
	"sync"

	"strconv"

	"github.com/antongulenko/go-bitflow-pipeline/query"
)

type SubprocessEngine struct {
	Executable string

	capabilities  query.ProcessingSteps
	pipelines     map[int]*RunningPipeline
	pipelinesLock sync.Mutex

	nextId     int
	nextIdLock sync.Mutex
}

type RunningPipeline struct {
	Id     int
	Script string
}

func (engine *SubprocessEngine) Run() (err error) {
	engine.pipelines = make(map[int]*RunningPipeline)

	engine.capabilities, err = LoadCapabilities(engine.Executable)
	if err != nil {
		return
	}

	// TODO prepare subprocess shell

	return nil
}

func (engine *SubprocessEngine) NewPipeline(script string) (*RunningPipeline, error) {
	pipe := &RunningPipeline{
		Id:     engine.getNextId(),
		Script: script,
	}
	engine.pipelinesLock.Lock()
	engine.pipelines[pipe.Id] = pipe
	engine.pipelinesLock.Unlock()
	err := pipe.Run()
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
	// TODO
	return nil
}

func (pipe *RunningPipeline) Kill() error {
	// TODO
	// TODO the killed pipeline is kept for future reference. Add an explicit delete operation to clean up old pipelines.
	return nil
}

func (pipe *RunningPipeline) GetOutput() ([]byte, error) {
	// TODO
	return []byte("THIS IS SOME MOCK OUTPUT FOR PIPE " + strconv.Itoa(pipe.Id)), nil
}
