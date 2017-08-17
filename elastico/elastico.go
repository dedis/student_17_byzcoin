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
	"sort"
	"math"

	"time"
	"strconv"
)

const (
	stateMining = iota
	stateConsensus
)

func init() {
	onet.GlobalProtocolRegister("Elastico", func(n *onet.TreeNodeInstance) (onet.ProtocolInstance, error) {
		return NewElastico(n)
	})
}

type Elastico struct{

	*onet.TreeNodeInstance
	nodeList []*onet.TreeNode
	index int
	state int
	stMutex sync.Mutex
	memberHashHexString chan string

	// the target for the PoW
	target big.Int
	// the global threshold of nodes needed computed from committee members count
	threshold int


	committeeSize int
	committeeBits int
	committeeCount int


	tempNewMemberMsg []NewMember
	// all the members that this node has produced
	members map[string]*Member
	mms sync.Mutex

	// the members in the directory committee
	directoryCommittee map[string]int
	dc sync.Mutex


	finishMsgCnt int
	fmc sync.Mutex


	hashChan chan big.Int
	// prtocol start channel
	startProtocolChan chan startProtocolChan
	// channel to receive other nodes PoWs
	newMemberChan chan newMemberChan
	//channel to receive committee members from directory members
	committeeMembersChan chan committeeMembersChan
	// channel to receive blocks if we have a final member
	blockToFinalCommitteeChan chan blockToFinalCommitteeChan
	// channels for pbft
	prePrepareChan      chan prePrepareChan
	prepareChan         chan prepareChan
	commitChan          chan commitChan
	prePrepareFinalChan chan prePrepareFinalChan
	prepareFinalChan    chan prepareFinalChan
	commitFinalChan     chan commitFinalChan

	// Channels for testing
	OnStart chan bool
}




type Member struct {
	//id hash of this member
	hashHexString string
	// committee number
	committeeNo int
	// members in this member's committee
	committeeMembers map[string]int
	cm sync.Mutex
	// members in the final committee
	finalCommitteeMembers map[string]int
	// block that the committee wants to reach consensus
	committeeBlock *blockchain.TrBlock
	// if member is in a final committee
	isFinal bool
	isLeader bool
	// map of committee number to committee members
	directory map[int](map[string]int)
	dmtx sync.Mutex
	// channel to send new received member to this diretory member
	memberToDirectoryChan chan *NewMember
	// directory messages that this member has received
	directoryMsgCnt int
	dmc sync.Mutex
	// the pbft of this NewMember
	tempPrepareMsg []*Prepare
	tempPrepareFinalMsg[] *PrepareFinal
	tempCommitMsg  []*Commit
	tempCommitFinalMsg []*CommitFinal
	tempBlockToFinalCommittee []*BlockToFinalCommittee
	// if the is the leader in the committee
	isLeaderChan chan bool
	// number of prepare messages received
	prepMsgCount int
	prepFinalMsgCount int
	// number of commit messages received
	commitMsgCount int
	commitFinalMsgCount int
	// thresholdPBFT which is 2.0/3.0 in pbft
	thresholdPBFT int
	// state that the member is in
	state int
	stMutex sync.Mutex
	// map of committee to block that they reached consensus
	finalConsensus map[int]string
	prePrepareChan chan bool
	prePrepareFinalChan chan *PrePrepareFinal
	prepareChan    chan *Prepare
	prepareFinalChan chan *PrepareFinal
	commitChan     chan *Commit
	commitFinalChan chan *CommitFinal
	finalBlockChan chan *BlockToFinalCommittee

	//
	committeeCnt int
}

func (els *Elastico) Setup () error{
	return nil
}

func NewElastico(n *onet.TreeNodeInstance) (*Elastico, error){
	els := new(Elastico)
	els.TreeNodeInstance = n
	els.nodeList = n.Tree().List()
	els.index = -1


	// Testing
	els.committeeCount = 2
	els.committeeBits = 1
	els.directoryCommittee = make(map[string]int)
	els.members = make(map[string]*Member)
	els.target = *big.NewInt(0).Exp(big.NewInt(2),big.NewInt(252),big.NewInt(0))   // FixMe 255 is Another variable
	els.committeeSize = 6
	//fmt.Print("target is" , els.target)
	//

	els.threshold = int (math.Ceil(float64(els.committeeSize) * 2.0 / 3.0))

	for i, tn := range els.nodeList{
		if tn.ID.Equal(n.TreeNode().ID){
			els.index = i
		}
	}

	//Test channels initialization
	els.OnStart = make(chan bool)
	//

	if els.index == -1 {
		panic(fmt.Sprint("Could not find ourselves %+v in the list of nodes %+v", n, els.nodeList))
	}
	//go els.checkFinish()
	// register the channels so the protocol calls appropriate functions upon receiving messages
	if err := n.RegisterChannel(&els.startProtocolChan); err != nil {
		return els, err
	}
	if err := n.RegisterChannel(&els.newMemberChan); err != nil {
		return els, err
	}
	if err := n.RegisterChannel(&els.committeeMembersChan); err != nil{
		return els, err
	}
	if err := n.RegisterChannel(&els.prePrepareChan); err != nil{
		return els,err
	}
	if err := n.RegisterChannel(&els.prepareChan); err != nil{
		return els, err
	}
	if err := n.RegisterChannel(&els.commitChan); err != nil {
		return els,err
	}
	if err := n.RegisterChannel(&els.prePrepareFinalChan); err != nil{
		return els, err
	}
	if err := n.RegisterChannel(&els.prepareFinalChan); err != nil{
		return els, err
	}
	if err := n.RegisterChannel(&els.commitFinalChan); err != nil{
		return els, err
	}
	if err := n.RegisterChannel(&els.blockToFinalCommitteeChan); err != nil{  // Adding this missed channel
		return els, err
	}



	return els, nil
}

func (els *Elastico) Start() error {
	log.LLvl5("Here is Start function of root")

	// broadcast to all nodes so that they start the protocol
	els.broadcast(func (tn *onet.TreeNode){
		if err := els.SendTo(tn, &StartProtocol{true}); err != nil {
			log.Error(els.Name(), "can't start protocol", tn.Name(), err)
		//	return err
		}

	})

//	els.OnStart <- true
	return nil
}


func (els *Elastico) Dispatch() error{
	for{
		select {
			case  <- els.startProtocolChan :
				els.handleStartProtocol()
			case msg := <- els.newMemberChan:
				els.handleNewMember(&msg.NewMember)
			case msg := <- els.committeeMembersChan :
				els.handleCommitteeMembers(&msg.CommitteeMembers)
			case msg := <- els.prePrepareChan :
				els.handlePrePrepare(&msg.PrePrepare)
			case msg := <- els.prepareChan :
				els.handlePrepare(&msg.Prepare)
			case msg := <- els.commitChan:
				els.handleCommit(&msg.Commit)
			case msg := <- els.blockToFinalCommitteeChan :
				els.handleBlockToFinalCommittee(&msg.BlockToFinalCommittee)
			case msg := <- els.prePrepareFinalChan :
				els.handlePrePrepareFinal(&msg.PrePrepareFinal)
			case msg := <- els.prepareFinalChan :
				els.handlePrepareFinal(&msg.PrepareFinal)
			case msg := <- els.commitFinalChan :
				els.handleCommitFinal(&msg.CommitFinal)
		}
	}
}

func (els *Elastico) handleStartProtocol() error {
	log.LLvl5("Handle Start Prtocol for number", els.index) // just for debugging
	els.state = stateMining
	go els.findPoW()
	return nil
}

func (els *Elastico) findPoW() {

	for i:=1; true;i++{
		time.Sleep(30 * time.Millisecond) // Simulating the process, Make it random time
	//	log.Print(i)

		els.stMutex.Lock()
		if els.state == stateMining {

			els.stMutex.Unlock()
			h := sha256.New()
			h.Write([]byte(strconv.Itoa(i + 785521 + els.index + rand.Int())))
			hashByte := h.Sum(nil)
			hashInt := new(big.Int).SetBytes(hashByte)
			if hashInt.Cmp(&els.target) == -1 {  // FIXME OR added for testing
				if err := els.handlePoW(hex.EncodeToString(hashByte)); err != nil{
					// FIXME handle errors with channels here
					return
				}
			}
		} else {
			els.stMutex.Unlock()
			return
		}
	}
}


func (els *Elastico) handlePoW (hashHexString string) error {
	els.dc.Lock()
	defer els.dc.Unlock()

	if len(els.directoryCommittee) < els.committeeSize {


		member := els.addMember(hashHexString)
		els.directoryCommittee[hashHexString] = els.index
	//	els.dc.Unlock()
		go els.runAsDirectory(member)
		els.broadcast(func(tn *onet.TreeNode) {
			if err := els.SendTo(tn,
				&NewMember{hashHexString, els.index}); err != nil {
				log.Error(els.Name(), "can't broadcast new member as directory", tn.Name(), err)
		//		return err
			}
		})
	} else {
		//els.dc.Unlock()
		els.addMember(hashHexString)
		els.broadcastToDirectory(func(tn *onet.TreeNode) {
			if err := els.SendTo(tn,
				&NewMember{hashHexString, els.index}); err != nil {
				log.Error(els.Name(), "can't broadcast new member to directories", tn.Name(), err)
		//		return err
			}
		})

	}
	//els.dc.Unlock()
	return nil
}


func (els *Elastico) handleNewMember (newMember *NewMember) error {
	//log.Print("New Member from node", newMember.NodeIndex, "is here in node", els.index) // Testing
	// check whether node's directory committee is complete :
	// if node's directory committee list is not complete and new member is not from this node,
	// then add it to node's directory committee list.
	// else if. it is from this node and node's directory committee list is not complete,
	// then this has been added before in handlePoW() method.

	els.dc.Lock()
	defer els.dc.Unlock()
	if len(els.directoryCommittee) < els.committeeSize {
		if els.index != newMember.NodeIndex {
			els.directoryCommittee[newMember.HashHexString] = newMember.NodeIndex
			return nil
		}
		return nil
	}
	//els.dc.Unlock()

	// node's directory list is complete
	for hashHexString, nodeIndex := range els.directoryCommittee {
		if nodeIndex == els.index {
			els.mms.Lock()
			directoryMember := els.members[hashHexString]
			els.mms.Unlock()

			go func() {directoryMember.memberToDirectoryChan <- newMember}()
			// directory committee nodes also has to know their committee
			for hashHexString, nodeIndex := range els.directoryCommittee {
				go func(t int, st string) {
					directoryMember.memberToDirectoryChan <- &NewMember{st, t}}(nodeIndex, hashHexString)
			}
		}
	}

	return nil
}


func (els *Elastico) runAsDirectory (directoryMember *Member) {
	log.Print("Member" , directoryMember.hashHexString, "Is Directory!") // Testing
	for {
		newMember := <- directoryMember.memberToDirectoryChan
	//	log.Print("newMember for directory", directoryMember.hashHexString) // Testing

		hashByte, _ := hex.DecodeString(newMember.HashHexString)
		committeeNo := els.getCommitteeNo(hashByte)
		if len(directoryMember.directory[committeeNo]) < els.committeeSize {
			//FIXME add some validation before accepting the id
			committeeMap := directoryMember.directory[committeeNo]
			committeeMap[newMember.HashHexString] = newMember.NodeIndex
		}
		completeCommittee := 0
		for i := 0 ; i < els.committeeCount ; i++ {
			if len(directoryMember.directory[i]) >= els.committeeSize {
				completeCommittee++
			}
		}
		if completeCommittee == els.committeeCount {
			if err := els.multicast(directoryMember); err != nil {
				//FIXME handle error here
			}
			return
		}
	}
	// FIXME maybe add this check later
	//if err != nil {
	//	log.Error("mis-formatted hash string")
	//	return err
	//}
}


func (els *Elastico) getCommitteeNo(bytes []byte) int {
	hashInt := new(big.Int).SetBytes(bytes)
	toReturn := 0
	for i := 0; i < els.committeeBits ; i++{
		if hashInt.Bit(i) == 1  {
			toReturn += 1 << uint(i)
		}
	}
//		log.Print(toReturn) // Testing
	return toReturn
}


func (els *Elastico) multicast(directoryMember *Member) error {
	finalCommittee := 0 // Its always committee number 0
	for committee, _:= range directoryMember.directory{
		for member, node := range directoryMember.directory[committee]{
			if err := els.SendTo(els.nodeList[node],
				&CommitteeMembers{
					directoryMember.directory[committee],
					directoryMember.directory[finalCommittee] , member}); err != nil{
				log.Error(els.Name(), "directory failed to send committee members", err)
				return err
			}
		}
	}
	return nil
}


func (els *Elastico) handleCommitteeMembers(committee *CommitteeMembers) error {
//	log.Print("Committee members sent for node" , els.index) // Test

	els.mms.Lock()
	memberToUpdate := els.members[committee.DestMember]
	els.mms.Unlock()
	for coMember, node := range committee.CoMembers {
		if memberToUpdate.hashHexString == coMember{
			continue
		}
		memberToUpdate.cm.Lock()
		memberToUpdate.committeeMembers[coMember] = node
		memberToUpdate.cm.Unlock()

	}
	for finMember, node := range committee.FinMembers {
		if memberToUpdate.hashHexString == finMember {
			memberToUpdate.isFinal = true
			continue
		}
		memberToUpdate.finalCommitteeMembers[finMember] = node
	}
	go els.checkForPBFT(memberToUpdate)
	return nil
}

func (els *Elastico) checkForPBFT(member *Member) error {
//	log.Print("member", member.hashHexString, "CheckPBFT func")
	member.dmc.Lock()
	member.directoryMsgCnt++
	if member.directoryMsgCnt >= els.threshold {
		member.directoryMsgCnt = 0
		member.dmc.Unlock()

		els.stMutex.Lock()
		if els.state == stateMining {
			els.state = stateConsensus
		}
		els.stMutex.Unlock()

		member.cm.Lock()
		newCm := member.committeeMembers
		member.cm.Unlock()

		leader := selectLeader(newCm)

		log.Print("Leader is", leader)
		if leader == member.hashHexString {
			member.isLeader = true
			go els.startPBFT(member)
		}
	//	close(member.isLeaderChan)  // I Commented this because there is no use of this channel
		if member.isLeader {
			member.cm.Lock()
			member.thresholdPBFT = int(math.Ceil(float64(len(member.committeeMembers))*2.0/3.0)) - 1
			member.cm.Unlock()

			member.stMutex.Lock()
			member.state = pbftStatePrepare
			member.stMutex.Unlock()
		} else {
			member.cm.Lock()
			member.thresholdPBFT = int(math.Ceil(float64(len(member.committeeMembers))*2.0/3.0))
			member.cm.Unlock()

			member.stMutex.Lock()
			member.state = pbftStatePrePrepare
			member.stMutex.Unlock()
		}
		return nil
	}

	member.dmc.Unlock()
	return nil
}

func selectLeader(committee map[string]int) string {
	var keys []string

	for member, _ := range committee{
		keys = append(keys, member)
	}
	sort.Strings(keys)
	return keys[0]
}


// here leader starts the inter-committee PBFT
func (els *Elastico) startPBFT(member *Member) {
	log.Print("memmber", member.hashHexString, "is Starting PBFT") // Test

	member.cm.Lock()
	defer member.cm.Unlock()
	for comMember, node := range member.committeeMembers{
		if err := els.SendTo(els.nodeList[node],
			&PrePrepare{member.committeeBlock, comMember}); err != nil{
			log.Error(els.Name(), "could not start PBFT", els.nodeList[node], node, els.index, err)
			return
		}
	}
	return
}


func (els *Elastico) handlePrePrepare(prePrepare *PrePrepare) error {
	log.Print("Node", els.index, "is on PrePrePare")

	els.mms.Lock()
	member := els.members[prePrepare.DestMember]
	els.mms.Unlock()

	member.stMutex.Lock()
	if member.state != pbftStatePrePrepare {
		member.stMutex.Unlock()
		// FIXME make this channel with buffer
		member.prePrepareChan <- true
		return nil
	}
	member.stMutex.Unlock()

	go els.handlePrePreparePBFT(member, prePrepare)
	member.prePrepareChan <- true
	return nil
}


func (els *Elastico) handlePrePreparePBFT(member *Member, prePrepare *PrePrepare) {
	<- member.prePrepareChan

	log.Print("Member" , member.hashHexString, "is on PrePrePare")
	member.stMutex.Lock()
	member.state = pbftStatePrepare
	member.stMutex.Unlock()

	member.committeeBlock = prePrepare.TrBlock
	go els.handlePreparePBFT(member)

	member.cm.Lock()
	for coMember, node := range member.committeeMembers {
		if err := els.SendTo(els.nodeList[node],
			&Prepare{member.committeeBlock.HeaderHash, coMember}); err != nil {
			log.Error(els.Name(), "can't send prepare message", els.nodeList[node].Name(), err)
		}
	}
	member.cm.Unlock()

	for _, prepare := range member.tempPrepareMsg {
		member.prepareChan <- prepare
	}
	member.tempPrepareMsg = nil
}


func (els *Elastico) handlePrepare(prepare *Prepare) error {
	log.Print("Node", els.index, "is on PrePare")

	els.mms.Lock()
	member := els.members[prepare.DestMember]
	els.mms.Unlock()

	member.stMutex.Lock()
	if member.state != pbftStatePrepare {
		member.stMutex.Unlock()
		member.tempPrepareMsg = append(member.tempPrepareMsg, prepare)
		return nil
	}
	member.stMutex.Unlock()

	member.prepareChan <- prepare
	return nil
}


func (els *Elastico) handlePreparePBFT(member *Member) {
	log.Print("Member" , member.hashHexString, "is on PrePare")
	for {
		<- member.prepareChan

		member.prepMsgCount++
		if member.prepMsgCount >= member.thresholdPBFT {
			log.Print("Member", member.hashHexString, "Reached prepare threshold")
			member.prepMsgCount = 0
			member.stMutex.Lock()
			member.state = pbftStateCommit
			member.stMutex.Unlock()

			go els.handleCommitPBFT(member)
			for coMember, node := range member.committeeMembers {
				if err := els.SendTo(els.nodeList[node],
					&Commit{member.committeeBlock.HeaderHash, coMember}); err != nil {
					log.Error(els.Name(), "can't send prepare message", els.nodeList[node].Name(), err)
				}
			}
			for _, commit := range member.tempCommitMsg {
				member.commitChan <- commit
			}
			member.tempCommitMsg = nil
			return
		}
	}
}

func (els *Elastico) handleCommit(commit *Commit) error{
	log.Print("Commit sended to node" , els.index)

	els.mms.Lock()
	member := els.members[commit.DestMember]
	els.mms.Unlock()

	member.stMutex.Lock()
	if member.state != pbftStateCommit {
		member.stMutex.Unlock()
		member.tempCommitMsg = append(member.tempCommitMsg, commit)
		return nil
	}
	member.stMutex.Unlock()

	member.commitChan <- commit
	return nil
}


func (els *Elastico) handleCommitPBFT(member *Member) {
	for {
		<- member.commitChan
		log.Print("new Commit for member", member.hashHexString)
		member.commitMsgCount++
		if member.commitMsgCount >= member.thresholdPBFT {
			log.Print(member.hashHexString, "Reached COMMIT threshold")
			member.commitMsgCount = 0
			if member.isFinal{
				member.stMutex.Lock()
				member.state = pbftStatePrePrepareFinal
				member.stMutex.Unlock()

				go els.finalPBFT(member)
				for _, block := range member.tempBlockToFinalCommittee{
					member.finalBlockChan <- block
				}
			} else {
				els.broadcastToFinal(member)
			}
		}
	}
}


func (els *Elastico) broadcastToFinal (member *Member) {
	log.Print("Final broadcast of member", member.hashHexString)
	for finMember, node := range member.finalCommitteeMembers {
		if err := els.SendTo(els.nodeList[node],
			&BlockToFinalCommittee{member.committeeBlock.HeaderHash,
								   finMember, member.committeeNo}); err != nil {
			log.Error(els.Name(), "can't send block to final committee", els.nodeList[node], err)
			return
		}
	}
	member.stMutex.Lock()
	member.state = pbftStateFinish
	member.stMutex.Unlock()

	go els.checkFinish()
}


func (els *Elastico) handleBlockToFinalCommittee(block *BlockToFinalCommittee) {
	els.mms.Lock()
	member := els.members[block.DestMember]
	els.mms.Unlock()

	if member.state != pbftStatePrePrepareFinal {
		member.tempBlockToFinalCommittee = append(member.tempBlockToFinalCommittee, block)
		return
	}
	member.finalBlockChan <- block
}


func (els *Elastico) finalPBFT(member *Member) {
	for {
		block := <- member.finalBlockChan
		member.finalConsensus[block.committeeNo] = block.HeaderHash
		if len(member.finalConsensus) == els.committeeCount {
			if member.isLeader {
				member.state = pbftStatePrepareFinal
				go els.handlePrepareFinalPBFT(member)
				for coMember, node := range member.committeeMembers{
					if err := els.SendTo(els.nodeList[node],
						&PrePrepareFinal{member.finalConsensus[0], coMember}); err != nil{
						log.Error(els.Name(), "can't start final consensus", els.nodeList[node], err)
					}
				}
			}
		}
	}
}

func (els *Elastico) handlePrePrepareFinal(prePrepareFinal *PrePrepareFinal){
	els.mms.Lock()
	member := els.members[prePrepareFinal.DestMember]
	els.mms.Unlock()

	if member.state != pbftStatePrePrepareFinal {
		member.prePrepareFinalChan <- prePrepareFinal
		return
	}
	go els.handlePrePrepareFinalPBFT(member)
	member.prePrepareFinalChan <- prePrepareFinal
	return
}

func (els *Elastico) handlePrePrepareFinalPBFT(member *Member) {
	block := <- member.prePrepareFinalChan
	member.state = pbftStatePrepareFinal
	go els.handlePrepareFinalPBFT(member)
	for _, prepare := range member.tempPrepareFinalMsg {
		member.prepareFinalChan <- prepare
	}

	for coMember, node := range member.committeeMembers{
		if err := els.SendTo(els.nodeList[node],
			&PrepareFinal{block.HeaderHash, coMember}); err != nil{
			log.Error(els.Name(), "can't send preprepare in final pbft", els.nodeList[node], err)
		}
	}
}

func (els *Elastico) handlePrepareFinal(prepareFinal *PrepareFinal) {
	els.mms.Lock()
	member := els.members[prepareFinal.DestMember]
	els.mms.Unlock()

	if member.state != pbftStatePrepareFinal {
		member.tempPrepareFinalMsg = append(member.tempPrepareFinalMsg, prepareFinal)
		return
	}
	member.prepareFinalChan <- prepareFinal
}

func (els *Elastico) handlePrepareFinalPBFT(member *Member){
	for {
		block := <- member.prepareFinalChan
		member.prepFinalMsgCount++
		if member.prepFinalMsgCount >= member.thresholdPBFT {
			member.prepFinalMsgCount = 0
			member.state = pbftStateCommitFinal
			go els.handleCommitFinalPBFT(member)
			for _, commit := range member.tempCommitFinalMsg {
				member.commitFinalChan <- commit
			}
			member.tempCommitFinalMsg = nil
			for coMember, node := range member.committeeMembers {
				if err := els.SendTo(els.nodeList[node],
					&CommitFinal{block.HedearHash, coMember}); err != nil{
					log.Error(els.Name(), "can't send to member in final commit", els.nodeList[node], err)
				}
			}
			return
		}
	}
}

func (els *Elastico) handleCommitFinal(commitFinal *CommitFinal) {
	els.mms.Lock()
	member := els.members[commitFinal.DestMember]
	els.mms.Unlock()

	if member.state != pbftStateCommitFinal {
		member.tempCommitFinalMsg = append(member.tempCommitFinalMsg, commitFinal)
	}
	member.commitFinalChan <- commitFinal
}

func (els *Elastico) handleCommitFinalPBFT(member *Member) {
	for {
		<- member.commitFinalChan
		member.commitFinalMsgCount++
		if member.commitFinalMsgCount >= member.thresholdPBFT {
			member.commitFinalMsgCount = 0
			member.state = pbftStateFinish
			go els.checkFinish()
			return
		}
	}
}

func (els *Elastico) checkFinish() {
	for {
		els.fmc.Lock()
		els.finishMsgCnt++
		var pointlessMembers int = 0

		els.mms.Lock()
		ln := len(els.members)
		for _, member := range els.members {
			if member.state == pbftStateNotReady {
				pointlessMembers++
			}
		}
		els.mms.Unlock()

		log.Print("finished members are", els.finishMsgCnt)
		nodeMembersCnt := ln - pointlessMembers
		if els.finishMsgCnt == nodeMembersCnt {
			log.Print("The End!")
			els.Done()
			return
		}
		els.fmc.Unlock()
	}

}

func (els *Elastico) addMember (hashHexString string) *Member{
//	log.LLvl5("New member for node", els.index, "hash :" , hashHexString) // Debug

	member := new(Member)


	member.hashHexString = hashHexString
	member.state = pbftStateNotReady
	member.memberToDirectoryChan = make(chan *NewMember)
	member.finalConsensus = make(map[int]string)
	member.prePrepareChan  = make(chan bool,1)
	member.prePrepareFinalChan =  make(chan *PrePrepareFinal)
	member.prepareChan =     make(chan *Prepare)
	member.prepareFinalChan =  make(chan *PrepareFinal)
	member.commitChan  = make(chan *Commit)
	member.commitFinalChan =  make(chan *CommitFinal)
	member.finalBlockChan =  make(chan *BlockToFinalCommittee)
	member.isLeaderChan = make(chan bool)
	member.committeeMembers = make(map[string]int)
	member.committeeMembers[member.hashHexString] = els.index // Every one is in its committee

	// Setting Committee Number for this node
	hashByte, _ := hex.DecodeString(member.hashHexString)
	member.committeeNo = els.getCommitteeNo(hashByte)

	//Testing , Assigning a block to this member, Fixme : Returning error
	err := blockchain.EnsureBlockIsAvailable("/home/javad/go/src/github.com/dedis/student_17_byzcoin/elastico")
	if err != nil {
		log.Fatal("Couldn't get block:", err)
	}
	var magicNum = [4]byte{0xF9, 0xBE, 0xB4, 0xD9}
	dir := blockchain.GetBlockDir()
	parser, err := blockchain.NewParser(dir, magicNum)
	if err != nil {
		log.Error("Error: Couldn't parse blocks in", dir)
	}
	transactions, err := parser.Parse(0, 0)
	if err != nil {
		log.Error("Error while parsing transactions", err)
	}

	trlist := blockchain.NewTransactionList(transactions, len(transactions))
	header := blockchain.NewHeader(trlist, "", "")
	trblock := blockchain.NewTrBlock(trlist, header)
	member.committeeBlock = trblock
	//

	member.finalCommitteeMembers = make(map[string]int)
	member.directory = make(map[int](map[string]int))
	member.committeeCnt = 2 // FIXME must be committeeCnt of Root

	for i:=0; i<member.committeeCnt; i++ {
		member.directory[i] = make(map[string]int)
	}


	els.mms.Lock()
	els.members[hashHexString] = member
	els.mms.Unlock()

	return member
}


func (els *Elastico) broadcast(sendCB func(node *onet.TreeNode) ) {
	for _ ,node := range els.nodeList{
		go sendCB(node)
	}
}

func (els *Elastico) broadcastToDirectory(sendCB func(node *onet.TreeNode) ) {
	//els.dc.Lock()
	for _, node := range els.directoryCommittee{
		go sendCB(els.nodeList[node])
	}
	//els.dc.Unlock()
}
func  (els *Elastico) PrintStatus () {
	log.Print("Target is : " , els.target)
	log.Print("Threshold is" , els.threshold)

	log.Print("members of this node :")
	for str, member := range els.members {
		log.Print(str)
		member.PrintStatus()
	}

	log.Print("directory committees :")
	for member,_ := range els.directoryCommittee {
		log.Print(member)
	}
}
func (member *Member) PrintStatus() {
	log.Print("Printing a Member Status", member.hashHexString)
	log.Print("Committe Number of this node : ", member.committeeNo)

	log.Print("Committee Members :")
	for str,_  := range member.committeeMembers {
		log.Print(str)
	}
	log.Print("Final committee members of this node  are :")
	for str,_  := range member.finalCommitteeMembers {
		log.Print(str)
	}



}
