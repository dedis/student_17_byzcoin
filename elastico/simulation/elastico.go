package main

import (
	"gopkg.in/dedis/onet.v1"
	"github.com/dedis/student_17_byzcoin/elastico"
	"github.com/BurntSushi/toml"
	"github.com/dedis/cothority/byzcoin/blockchain"
	"gopkg.in/dedis/onet.v1/log"
	"github.com/dedis/cothority/byzcoin/blockchain/blkparser"
	"github.com/dedis/cothority/messaging"
	"gopkg.in/dedis/onet.v1/simul/monitor"
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
	var rootNodeBlocks []*blockchain.TrBlock
	for i := 0; i < config.Tree.Size() ; i++ {
		var transactions []blkparser.Tx
		blk, _ := chain.NextBlock()
		for _ , tx := range blk.Txs {
			transactions = append(transactions, *tx)
		}
		trList :=  blockchain.NewTransactionList(transactions, len(transactions))
		header := blockchain.NewHeader(trList, "", "")
		trblock := blockchain.NewTrBlock(trList, header)
		rootNodeBlocks = append(rootNodeBlocks, trblock)
	}

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

	log.Lvl1("Simulation can start!")
	for round := 0; round < e.Rounds; round++ {
		log.Lvl1("starting round", round)
		p, err := config.Overlay.CreateProtocol("Elastico", config.Tree, onet.NilServiceID)
		if err != nil {
			return err
		}
		els := p.(*elastico.Elastico)
		els.OnDoneCB  = onDoneCB
		els.CommitteeCount = e.CommitteeCount
		els.CommitteeSize = e.CommitteeSize
		els.TargetBit = e.Target
		for _, trBlock := range rootNodeBlocks {
			els.RootNodeBlocks = append(els.RootNodeBlocks, trBlock)
		}
		r := monitor.NewTimeMeasure("round-elastico")
		if err := els.Start(); err != nil {
			log.Error("Couldn't start elastico")
			return err
		}
		<- doneChan
		r.Record()
		log.Lvl1("finished round", round)
	}
	return nil
}








