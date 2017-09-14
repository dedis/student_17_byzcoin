package main_test

import (
	"testing"

	"gopkg.in/dedis/onet/simul"
)

func TestSimulation(t *testing.T) {
	simul.Start("byzcoin.toml", "ntree.toml", "pbft.toml")
}
