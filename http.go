package main

import (
	"io/ioutil"
	"net/http"

	"fmt"

	"strconv"

	log "github.com/Sirupsen/logrus"
	"github.com/antongulenko/go-bitflow-pipeline/http"
	"github.com/gin-gonic/gin"
)

func (engine *SubprocessEngine) ServeHttp(endpoint string) error {
	g := plotHttp.NewGinEngine()
	g.GET("/capabilities", engine.serveCapabilities)
	g.GET("/pipelines", engine.servePipelines)
	g.POST("/pipeline", engine.serveNewPipeline)
	g.GET("/pipeline/:id", engine.serveGetPipeline)
	g.GET("/pipeline/:id/out", engine.serveGetPipelineOutput)
	g.DELETE("/pipeline/:id", engine.serveKillPipeline)
	return g.Run(endpoint)
}

func (engine *SubprocessEngine) replyString(c *gin.Context, code int, format string, args ...interface{}) {
	c.Status(code)
	c.Writer.WriteString(fmt.Sprintf(format+"\n", args...))
}

func (engine *SubprocessEngine) serveCapabilities(c *gin.Context) {
	c.JSON(http.StatusOK, engine.capabilities)
}

func (engine *SubprocessEngine) servePipelines(c *gin.Context) {
	engine.pipelinesLock.Lock()
	response := make([]int, 0, len(engine.pipelines))
	for id := range engine.pipelines {
		response = append(response, id)
	}
	engine.pipelinesLock.Unlock()
	c.JSON(http.StatusOK, response)
}

func (engine *SubprocessEngine) pipelineResponse(pipe *RunningPipeline) interface{} {
	// TODO maybe don't serve the entire internal struct?
	return pipe
}

func (engine *SubprocessEngine) serveNewPipeline(c *gin.Context) {
	defer func() {
		err := c.Request.Body.Close()
		if err != nil {
			log.Warnln("Error closing POST request body:", err)
		}
	}()

	script, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		engine.replyString(c, http.StatusInternalServerError, "Failed to read request body: "+err.Error())
		return
	}

	if len(script) == 0 {
		engine.replyString(c, http.StatusBadRequest, "Provide the Bitflow script for the new pipeline as the POST request body.")
		return
	}

	pipeline, err := engine.NewPipeline(string(script))
	if err != nil {
		engine.replyString(c, http.StatusBadRequest, "Error starting pipeline %v: %v", pipeline.Id, err.Error())
	} else {
		c.JSON(http.StatusCreated, engine.pipelineResponse(pipeline))
	}
}

func (engine *SubprocessEngine) serveGetPipeline(c *gin.Context) {
	pipe := engine.getPipeline(c)
	if pipe != nil {
		c.JSON(http.StatusOK, engine.pipelineResponse(pipe))
	}
}

func (engine *SubprocessEngine) serveGetPipelineOutput(c *gin.Context) {
	pipe := engine.getPipeline(c)
	if pipe != nil {
		out, err := pipe.GetOutput()
		if err == nil {
			c.Status(http.StatusOK)
			c.Writer.Write(out)
		} else {
			engine.replyString(c, http.StatusInternalServerError, "Error obtaining output of pipeline %v", pipe.Id)
		}
	}
}

func (engine *SubprocessEngine) serveKillPipeline(c *gin.Context) {
	pipe := engine.getPipeline(c)
	if pipe != nil {
		err := pipe.Kill()
		if err != nil {
			engine.replyString(c, http.StatusInternalServerError, "Error killing pipeline %v: %v", pipe.Id, err)
		} else {
			engine.replyString(c, http.StatusOK, "Pipeline %v has been killed", pipe.Id)
		}
	}
}

func (engine *SubprocessEngine) getPipeline(c *gin.Context) *RunningPipeline {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		engine.replyString(c, http.StatusBadRequest, "Failed to parse parameter '%v' to int: %v", idStr, err)
	}

	engine.pipelinesLock.Lock()
	pipeline, exists := engine.pipelines[id]
	engine.pipelinesLock.Unlock()
	if !exists {
		engine.replyString(c, http.StatusNotFound, "Pipeline does not exist: "+idStr)
	}
	return pipeline
}
