package main

import (
	"gopkg.in/dedis/onet.v1/simul/monitor"
	"github.com/dedis/student_17_byzcoin/elastico"
	"github.com/BurntSushi/toml"
	"github.com/dedis/cothority/byzcoin/blockchain"
	"gopkg.in/dedis/onet.v1/log"
	"github.com/dedis/cothority/byzcoin/blockchain/blkparser"
	"github.com/dedis/cothority/messaging"
	"gopkg.in/dedis/onet.v1"
	"os"
	"fmt"
	"time"
)


var magicNum = [4]byte{0xF9, 0xBE, 0xB4, 0xD9}


func init() {
	onet.SimulationRegister("Elastico", NewElasticoSimulation)
	onet.GlobalProtocolRegister("Elastico", func(n *onet.TreeNodeInstance) (onet.ProtocolInstance, error) {
		return elastico.NewElastico(n)
	})
}



type ElasticoSimulation struct {
	onet.SimulationBFTree
	// Elastico simulation specific fields:
	// number of committees
	CommitteeCount int
	// size of each committee
	CommitteeSize int
	// block size is the number of transactions in one block
	BlockSize int
	// target of PoW
	Target int
}

func NewElasticoSimulation(config string) (onet.Simulation, error){
	sim := &ElasticoSimulation{}
	_, err := toml.Decode(config, sim)
	if err != nil {
		return nil, err
	}
	return sim, nil
}

// in setup the tree and the roster get initialized. also by using EnsureBlockIsAvailable one .dat file
// is fetched from pop.dedis.ch and saved in your build directory inside the simulation directory.
func (e *ElasticoSimulation) Setup(dir string, hosts []string) (*onet.SimulationConfig, error) {
	err := blockchain.EnsureBlockIsAvailable(dir)
	if err != nil {
		log.Fatal("Couldn't get block:", err)
	}
	sc := &onet.SimulationConfig{}
	e.CreateRoster(sc, hosts, 2000)
	err = e.CreateTree(sc)
	if err != nil {
		return nil, err
	}
	return sc, nil
}


func (e *ElasticoSimulation) Run (config *onet.SimulationConfig) error {
	dir := blockchain.GetBlockDir()
	chain , _ := blkparser.NewBlockchain(dir, magicNum)
	var rootNodeBlock *blockchain.TrBlock
	var transactions []blkparser.Tx
	for j := 0; j < e.BlockSize; j++{
		blk, _ := chain.NextBlock()
		for _ , tx := range blk.Txs {
			transactions = append(transactions, *tx)
		}
	}
	trList :=  blockchain.NewTransactionList(transactions, len(transactions))
	header := blockchain.NewHeader(trList, "", "")
	rootNodeBlock = blockchain.NewTrBlock(trList, header)



	pi, err := config.Overlay.CreateProtocol("Broadcast", config.Tree, onet.NilServiceID)
	if err != nil {
		log.Error(err)
	}
	proto, _ := pi.(*messaging.Broadcast)
	// channel to notify we are done
	broadDone := make(chan bool)
	proto.RegisterOnDone(func() {
		broadDone <- true
	})

	// ignore error on purpose: Start always returns nil
	_ = proto.Start()

	// wait
	<-broadDone

	doneChan := make(chan bool)
	onDoneCB := func(){
		doneChan <- true
	}
	var pbftStartTime time.Time
	measureStartTime := func (){
		pbftStartTime = time.Now()
	}
	var pbftFinishTime time.Time
	measureFinishTime := func (){
		pbftFinishTime = time.Now()
	}

	log.Lvl1("Simulation can start!")
	//FIXME this file is written on deterlab gateway. if you want to have it locally you have to deal with onet.
	f, err := os.OpenFile("info.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Error(err)
	}
	defer f.Close()
	size := uint32(0)
	for _, tx := range rootNodeBlock.Txs {
		size += tx.Size
	}
	f.WriteString(fmt.Sprintf("%d ", e.CommitteeCount) +
					fmt.Sprintf("%d ", e.CommitteeSize) +
					fmt.Sprintf("%d\r", size))
	for round := 0; round < e.Rounds; round++ {
		// the simulation can start
		log.Lvl1("starting round", round)
		p, err := config.Overlay.CreateProtocol("Elastico", config.Tree, onet.NilServiceID)
		if err != nil {
			return err
		}
		els := p.(*elastico.Elastico)
		els.OnDoneCB  = onDoneCB
		els.MeasureStartTime = measureStartTime
		els.MeasureFinishTime = measureFinishTime
		els.CommitteeCount = e.CommitteeCount
		els.CommitteeSize = e.CommitteeSize
		els.TargetBit = e.Target
		els.RootNodeBlock = rootNodeBlock
		r := monitor.NewTimeMeasure("round-elastico")
		if err := els.Start(); err != nil {
			log.Error("Couldn't start elastico")
			return err
		}
		<- doneChan
		elapsed := pbftFinishTime.Sub(pbftStartTime).Seconds()
		f.WriteString(fmt.Sprintf("%f\r", elapsed))
		r.Record()
		log.Lvl1("finished round", round)
	}
	return nil
}