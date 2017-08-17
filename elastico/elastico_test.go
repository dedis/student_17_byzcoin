package elastico_test


import (
	"github.com/dedis/student_17_byzcoin/elastico"
	"gopkg.in/dedis/onet.v1"
	"gopkg.in/dedis/onet.v1/log"
	"testing"
	//"time"
	"time"
)

func TestElastico(t *testing.T) {
	numHosts := 5
	log.Print("Running elastico with", numHosts, "Hosts")

	localTest := onet.NewLocalTest()
	_, _, tree := localTest.GenBigTree(numHosts, numHosts, 3, true)

	pr, err := localTest.CreateProtocol("Elastico", tree)
	if (err != nil) {
		t.Errorf("Can not Create protocol")
	}


	root := pr.(*elastico.Elastico)

	go root.Start()

	log.LLvl5("Start Function called")

	/*
	select {
	case <- (root.OnStart):
		log.Lvl3("Protocol Successfully Started")
	case <-time.After(40 * time.Second):
		t.Errorf("20 Seconds and No responds, protocol failed")
	}
	*/
	time.Sleep(7*time.Second)
	root.PrintStatus()

	t.Errorf("Fake error")

	localTest.CloseAll();

}