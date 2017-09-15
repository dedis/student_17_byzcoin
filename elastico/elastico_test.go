package elastico

import (
	"gopkg.in/dedis/onet.v1"
	"gopkg.in/dedis/onet.v1/log"
	"testing"
	"time"
	"github.com/dedis/cothority/byzcoin/blockchain/blkparser"
	"github.com/dedis/cothority/byzcoin/blockchain"
)


func TestElastico(t *testing.T) {
	numHosts := 20
	log.Lvl2("Running elastico with", numHosts, "Hosts")

	localTest := onet.NewLocalTest()
	_, _, tree := localTest.GenBigTree(numHosts, numHosts, 3, true)

	pr, err := localTest.CreateProtocol("Elastico", tree)
	if (err != nil) {
		t.Errorf("Can not Create protocol")
	}


	root := pr.(*Elastico)

	var magicNum = [4]byte{0xF9, 0xBE, 0xB4, 0xD9}
	dir := blockchain.GetBlockDir()
	chain , _ := blkparser.NewBlockchain(dir, magicNum)
	var rootNodeBlocks []*blockchain.TrBlock
	for i := 0; i < 20 ; i++ {   // 20 is an upper bound for nodes or members
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

	doneChan := make(chan bool)
	onDoneCB := func(){
		doneChan <- true
	}
	root.OnDoneCB  = onDoneCB
	root.CommitteeCount = 16
	root.CommitteeSize = 20
	root.TargetBit = 3
	root.RootNodeBlock = rootNodeBlocks[0]

	var pbftStartTime time.Time
	measureStartTime := func (){
		pbftStartTime = time.Now()
	}
	var pbftFinishTime time.Time
	measureFinishTime := func (){
		pbftFinishTime = time.Now()
	}
	root.MeasureStartTime = measureStartTime
	root.MeasureFinishTime = measureFinishTime



	go root.Start()

	select {
	case <- doneChan :
		t.Log("Protocol finished successfully")
	case <-time.After(100 * time.Second):
		t.Errorf("100 Seconds and No responds, protocol failed")
	}
	localTest.CloseAll();

}
