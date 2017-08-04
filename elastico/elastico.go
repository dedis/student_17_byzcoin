package elastico


import (
	"gopkg.in/dedis/onet.v1"
	"github.com/dedis/cothority/byzcoin/blockchain"
	"fmt"
	"sync"
	"math/big"
	"gopkg.in/dedis/onet.v1/log"
	"crypto/sha256"
	"math/rand"
	"encoding/hex"
)

const (
	c = 100
	target
	stateMining
	stateIntra
)


type Elastico struct{
	// the node we represent in
	*onet.TreeNodeInstance
	// nodes in the tree
	nodeList []*onet.TreeNode
	// our index in the tree
	index int
	// our state in the protocol
	state int

	// all the POWs that this node has produced
	ids map[string]*IDsPoW
	idm sync.Mutex
	// the nodes in the directory committee
	directoryCommittee map[string]int
	dc sync.Mutex
	// the nodes in the final committee
	finalCommittee map[string]int
	fc	sync.Mutex


	hashChan chan big.Int

	// prtocol start channel
	startProtocolChan chan startProtocolChan
	// channel to receive other nodes PoWs
	idChan chan idChan
	// channel to receive group committee ids from directory
	groupCommitteeIDChan chan groupCommitteeIDChan
	// channel to receive final committee ids from directory
	finalCommitteeChan chan finalCommitteeChan
	// channel to receive the block in the group
	finalBlockChan chan finalBlockChan
	// channel to receive random string
	randomStringChan chan randomStringChan

	// channels for pbft
	prePrepareChan chan prePrepareChan
	prepareChan    chan prepareChan
	commitChan     chan commitChan
	finishChan     chan finishChan
}

type IDsPoW struct {
	// the id that specifies pow's group
	id string

	// the ids with the same committee with this id
	groupCommitteeID map[string]int

	// block that the group must agree on
	groupBlock blockchain.TrBlock

	// if the id is final
	isFinal bool
	// the random string to generate
	randomString string
	// the random set to offer to next round
	randomSet []string
	rs sync.Mutex

	// if the node is directory
	isDirecotory bool
	// map of group number to group member indices
	dir map[int]([]int)

	// the pbft of this ID
	tempPrepareMsg []Prepare
	tpm sync.Mutex
	tempCommitMsg  []Commit
	tcm sync.Mutex
	// if the is the leader in the committee
	isLeader bool
	// number of prepare messages received
	prepMsgCount int
	pmc          sync.Mutex
	// number of commit messages received
	commitMsgCount int
	cmc            sync.Mutex
	// threshold which is 2.0/3.0 in pbft
	threshold int
	// state that the node is in
	state int
}

func NewElastico(n *onet.TreeNodeInstance) (*Elastico, error){
	els := new(Elastico)
	els.TreeNodeInstance = n
	els.nodeList = n.Tree().List()
	els.index = -1
	for i, tn := range els.nodeList{
		if tn.ID.Equal(n.TreeNode().ID){
			els.index = i
		}
	}
	if els.index == -1 {
		panic(fmt.Sprint("Could not find ourselves %+v in the list of nodes %+v", n, els.nodeList))
	}

	// register the channels so the protocol calls appropriate functions upon receiving messages
	if err := n.RegisterChannel(&els.startProtocolChan); err != nil {
		return els, err
	}
	if err := n.RegisterChannel(&els.idChan); err != nil {
		return els, err
	}
	if err := n.RegisterChannel(&els.groupCommitteeIDChan); err!= nil{
		return els, err
	}
	if err := n.RegisterChannel(&els.finalCommitteeChan); err != nil {
		return els, err
	}
	if err := n.RegisterChannel(&els.finalBlockChan); err != nil{
		return els,err
	}
	if err := n.RegisterChannel(&els.randomStringChan); err != nil{
		return els, err
	}
	return els, nil
}


func (els *Elastico) Start() error{
	els.broadcast(func (tn *onet.TreeNode){
		if err := els.SendTo(tn, &startProtocol{true}); err != nil {
			panic(fmt.Sprintf("The protocol can't start"))
		}
	})
	return nil
}

func (els *Elastico) Dispatch() error{
	for{
		select {
			case  <- els.startProtocolChan :
				els.startPoW()
		}
	}
}


func (els *Elastico) startPoW() {
	els.state = stateMining
	go els.computePoW()
}



func (els *Elastico) computePoW() {
	// FIXME make the mining process more random
	for {
		var hash big.Int
		if els.state == stateMining {
			h := sha256.New()
			h.Write([]byte(string(els.index + rand.Int())))
			shaHash := h.Sum(nil)
			hash.SetBytes(shaHash)
			// FIXME make target comparable to big.Int
			if hash.Cmp(target) < 0 {
				els.hashChan <- hash
			}
			hexHash := hex.EncodeToString(shaHash)
			els.dc.Lock()
			if len(els.directoryCommittee) < c {
				els.directoryCommittee[hexHash] = els.index
				els.ids[hexHash] = els.makeDirectoryID(hexHash)
				els.broadcast(func (tn *onet.TreeNode){
					if err := els.SendTo(tn, &id{hexHash, els.index}); err != nil{
						log.Error(els.Name(), "can't broadcast new id", tn.Name(), err)
					}
				})
			}else{
				els.ids[hexHash] = els.makeID(hexHash)
				els.broadcastDirectory(func (tn *onet.TreeNode){
					if err := els.SendTo(tn, &id{hexHash, els.index}); err!= nil{
						log.Error(els.Name(), "can't send new id to directory")
					}
				})
			}
			els.dc.Unlock()
		}
	}
}



func (els *Elastico) makeID(id string) *IDsPoW {
	pow := new(IDsPoW)
	pow.id = id
	pow.state = preprepare
	return pow
}


func (els *Elastico) makeDirectoryID(id string) *IDsPoW{
	pow := new(IDsPoW)
	pow.id = id
	pow.isDirecotory = true
	pow.state = preprepare
	return pow
}



func (els *Elastico) broadcast(sendCB func(node *onet.TreeNode)){
	for _,node := range els.nodeList{
		//if i == els.index {
		//	continue
		//}
		go sendCB(node)
	}
}

func (els *Elastico) broadcastDirectory(sendCB func(tn *onet.TreeNode)) {
	for _, node := range els.directoryCommittee{
		go sendCB(node)
	}
}