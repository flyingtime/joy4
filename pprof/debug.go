//+build debug

package pprof

import (
	"log"

	"github.com/google/gops/agent"
)

const (
	gopsAddr = ":8091"
)

func init() {
	if err := agent.Listen(agent.Options{Addr: gopsAddr, ShutdownCleanup: true}); err != nil {
		log.Fatalln(err)
	}
}
