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
	// committee size
	c = 100
	// number of committee bits
	s = 2
	// number of committees
	comcnt
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
	members map[string]*IDsPoW
	idm     sync.Mutex
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
	 //channel to receive group committee members from directory
	committeeMembersChanChan chan committeeMembersChan
	// channel to receive final committee members from directory
	//finalCommitteeChan chan finalCommitteeChan
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
	// the ID that specifies pow's group
	id string

	// the members with the same committee with this ID
	groupCommitteeID map[string]int

	// block that the group must agree on
	groupBlock blockchain.TrBlock

	// if the ID is final
	isFinal bool
	// the random string to generate
	randomString string
	// the random set to offer to next round
	randomSet []string
	rs sync.Mutex

	// if the node is directory
	isDirecotory bool
	// map of group number to group member indices
	directory map[int]([]ID)

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
	if err := n.RegisterChannel(&els.committeeMembersChanChan); err != nil{
		return els, err
	}
	//if err := n.RegisterChannel(&els.finalBlockChan); err != nil{
	//	return els,err
	//}
	//if err := n.RegisterChannel(&els.randomStringChan); err != nil{
	//	return els, err
	//}
	return els, nil
}


func (els *Elastico) Start() error{
	// broadcast to all nodes that they should start the protocol
	els.broadcast(func (tn *onet.TreeNode){
		if err := els.SendTo(tn, &StartProtocol{true}); err != nil {
			panic(fmt.Sprintf("The protocol can't start"))
		}
	})
	return nil
}


func (els *Elastico) Dispatch() error{
	for{
		select {
			case  <- els.startProtocolChan :
				els.handleStartProtocol()
			case msg := <- els.idChan :
				els.HandleNewID(msg.ID)
		}
	}
}


func (els *Elastico) handleStartProtocol() error {
	els.state = stateMining
	// FIXME make the mining process more random
	for {
	var hashInt big.Int
		if els.state == stateMining {
			h := sha256.New()
			h.Write([]byte(string(els.index + rand.Int())))
			hashByte := h.Sum(nil)
			hashInt.SetBytes(hashByte)
			// FIXME make target comparable to big.Int
			if hashInt.Cmp(target) < 0 {
				hashHexString := hex.EncodeToString(hashByte)
				els.dc.Lock()
				if len(els.directoryCommittee) < c {
					els.directoryCommittee[hashHexString] = els.index
					els.members[hashHexString] = els.makeDirectoryID(hashHexString)
					els.broadcast(func (tn *onet.TreeNode){
						if err := els.SendTo(tn, &ID{hashHexString, els.index}); err != nil{
							log.Error(els.Name(), "can't broadcast new ID", tn.Name(), err)
							return err
						}
					})
				} else{
					els.members[hashHexString] = els.makeID(hashHexString)
					els.broadcastDirectory(func (tn *onet.TreeNode){
						if err := els.SendTo(tn, &ID{hashHexString, els.index}); err!= nil{
							log.Error(els.Name(), "can't send new ID to directory")
							return err
						}
					})
				}
				els.dc.Unlock()
			}
		}
	}
}


func (els *Elastico) HandleNewID(newID ID) {
	if len(els.directoryCommittee) < c {
		if newID.NodeIndex != els.index {
			els.directoryCommittee[newID.CommitteeHash] = newID.NodeIndex
			els.members[newID.CommitteeHash] = els.makeDirectoryID(newID.CommitteeHash)
		}
	}
	for hashString , index := range els.directoryCommittee {
		if els.index == index {
			els.runAsDirectory(hashString, newID)
		}
	}
}


func (els *Elastico) runAsDirectory(hashString string, newID ID) {
	hashByte, err := hex.DecodeString(newID.CommitteeHash)
	if err != nil {
		log.Error("mis-formatted id")
	}
	dirMember := els.members[hashString]
	committeeNo := getCommitteeNo(hashByte)

	if len(dirMember.directory) < c {
		dirMember.directory[committeeNo] = append(dirMember.directory[committeeNo], newID)
	}

	completeCommittee := 0
	for i := 0 ; i < comcnt ; i++ {
		if len(dirMember.directory[i]) >= c {
			completeCommittee++
		}
	}
	if completeCommittee == comcnt {
		// maybe we should add error here
		for i, _:= range dirMember.directory{
			for _, send := range dirMember.directory[i]{
				if err := els.SendTo(send,
					&CommitteeMembers{
						dirMember.directory[i],
						send.CommitteeHash}); err != nil{
					log.Error(els.Name(), "directory failed to send committee members", err)
				}
			}
		}
	}
}


func (dirMember *IDsPoW)multicastCommitee() {

}

func getCommitteeNo(bytes []byte) int {
	hashInt := big.NewInt(bytes)
	toReturn := 0
	for i := 0; i < s ; i++{
		if hashInt.Bit(i) {
			toReturn += 1 << uint(i)
		}
	}
	return toReturn
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

func (els *Elastico) broadcastDirectory(sendCB func(node *onet.TreeNode)) {
	for _, node := range els.directoryCommittee{
		go sendCB(node)
	}
}

