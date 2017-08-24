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


type Elastico struct{

	*onet.TreeNodeInstance
	nodeList []*onet.TreeNode
	index int
	state int
	RootNodeBlocks []*blockchain.TrBlock
	block *blockchain.TrBlock

	// the Target for the PoW
	Target big.Int
	TargetBit int
	// the global threshold of nodes needed computed from committee members count
	threshold int


	CommitteeSize  int
	CommitteeCount int
	committeeBits  int
	finalCommittee int


	tempNewMemberMsg []NewMember
	// all the members that this node has produced
	members map[string]*Member
	mmtx sync.Mutex
	// the members in the directory committee
	directoryCommittee map[string]int
	dc sync.Mutex


	finishMsgCnt     int
	rootFinishMsgCnt int
	OnDoneCB         func()
	fmc              sync.Mutex


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
	finishChan 			chan FinishChan
	finishRootChan 		chan finishRootChan
}




type Member struct {
	//id hash of this member
	hashHexString string
	// committee number
	committeeNo int
	// members in this member's committee
	committeeMembers map[string]int
	committeeMembersCnt map[string]int
	// members in the final committee
	finalCommitteeMembers map[string]int
	finalCommitteeMembersCnt map[string]int
	// Block that the committee wants to reach consensus
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
	noDirMsg bool
	// the pbft of this NewMember
	tempPrepareMsg []*Prepare
	tempPrepareFinalMsg[] *PrepareFinal
	tempCommitMsg  []*Commit
	tempCommitFinalMsg []*CommitFinal
	tempBlockToFinalCommittee []*BlockToFinalCommittee

	startFinalPBFTChan chan bool
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
	// map of committee to Block that they reached consensus
	finalConsensus map[int]string
	prePrepareChan chan *PrePrepare
	startPrePrepareChan chan bool
	prePrepareFinalChan chan *PrePrepareFinal
	startPrePrepareFinalChan chan bool
	prepareChan    chan *Prepare
	startPrepareChan chan bool
	prepareFinalChan chan *PrepareFinal
	startPrepareFinalChan chan bool
	commitChan     chan *Commit
	startCommitChan chan bool
	commitFinalChan chan *CommitFinal
	startCommitFinalChan chan bool
	finalBlockChan chan *BlockToFinalCommittee
}



func NewElastico(n *onet.TreeNodeInstance) (*Elastico, error){
	els := new(Elastico)
	els.TreeNodeInstance = n
	els.nodeList = n.Tree().List()
	els.index = -1
	els.members = make(map[string]*Member)
	els.directoryCommittee = make(map[string]int)
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
	if err := n.RegisterChannel(&els.finishChan); err != nil{
		return els, err
	}
	if err := n.RegisterChannel(&els.blockToFinalCommitteeChan); err != nil{
		return els, err
	}
	if err := n.RegisterChannel(&els.finishRootChan); err != nil{
		return els, err
	}
	return els, nil

}


func (els *Elastico) Start() error {
	// FIXME print stuff
	log.Print("root has started protocol")
	// broadcast to all nodes so that they start the protocol
	finalCommittee := rand.Intn(els.CommitteeCount)
	for index, node := range els.nodeList {
		sp := &StartProtocol{els.RootNodeBlocks[index],
							 els.CommitteeCount,
							 els.CommitteeSize,
							 finalCommittee,
							 els.TargetBit}
		els.SendTo(node, sp)
	}
	return nil
}


func (els *Elastico) Dispatch() error{
	for{
		select {
			case  msg := <- els.startProtocolChan :
				els.handleStartProtocol(&msg.StartProtocol)
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
			case <- els.finishChan :
				els.handleFinish()
			case <- els.finishRootChan :
				els.rootFinishMsgCnt++
				log.Lvl2("root has received", els.rootFinishMsgCnt, "finish messages")
				if els.rootFinishMsgCnt == len(els.nodeList){
					els.OnDoneCB()
					els.Done()
				}
		}
	}
}

func (els *Elastico) handleStartProtocol(start *StartProtocol) error {
	els.CommitteeCount = start.CommitteeCount
	els.CommitteeSize = start.CommitteeSize
	els.committeeBits = int (math.Log2(float64(els.CommitteeCount)))
	els.finalCommittee = start.FinalCommittee
	els.threshold = int (math.Ceil(float64(els.CommitteeSize)*2.0/3.0))
	els.block = start.Block
	els.state = stateMining
	els.Target = *big.NewInt(0).Exp(big.NewInt(2), big.NewInt(256 - int64(start.Target)), big.NewInt(0))
	// FIXME print stuff
	log.Print("node", els.index, "has started")
	go els.findPoW()
	return nil
}

func (els *Elastico) findPoW() {
	rand.Seed(int64(els.index))
	for i := 0; true; i++ {
		// simulate mining process
		time.Sleep(100 * time.Millisecond)
		if els.state == stateMining {
			h := sha256.New()
			h.Write([]byte(strconv.Itoa(rand.Int() + i)))
			hashByte := h.Sum(nil)
			hashInt := new(big.Int).SetBytes(hashByte)
			if hashInt.Cmp(&els.Target) == -1 {
				if err := els.handlePoW(hex.EncodeToString(hashByte)); err != nil{
					// FIXME handle errors with channels here
					return
				}
			}
		} else {
			return
		}
	}
}


func (els *Elastico) handlePoW (hashHexString string) error {
	els.dc.Lock()
	defer els.dc.Unlock()
	//log.Print("node", els.index, "has found", hashHexString)
	if len(els.directoryCommittee) < els.CommitteeSize {
		member := els.addMember(hashHexString)
		els.directoryCommittee[hashHexString] = els.index
		els.broadcast(func(tn *onet.TreeNode) {
			if err := els.SendTo(tn,
				&NewMember{hashHexString, els.index}); err != nil {
				log.Error(els.Name(), "can't broadcast new member as directory", tn.Name(), err)
				//return err
			}
		})
		go els.runAsDirectory(member)
	} else {
		els.addMember(hashHexString)
		els.broadcastToDirectory(func(tn *onet.TreeNode) {
			if err := els.SendTo(tn,
				&NewMember{hashHexString, els.index}); err != nil {
				log.Error(els.Name(), "can't broadcast new member to directory", tn.Name(), err)
				//return err
			}
		})
	}
	return nil
}


func (els *Elastico) handleNewMember (newMember *NewMember) error {
	// check whether node's directory committee is complete :
	// if node's directory committee list is not complete and new member is not from this node,
	// then add it to node's directory committee list.
	// else if it is from this node and node's directory committee list is not complete,
	// then this has been added before in handlePoW() method.
	els.dc.Lock()
	defer els.dc.Unlock()
	if len(els.directoryCommittee) < els.CommitteeSize {
		if els.index != newMember.NodeIndex {
			els.directoryCommittee[newMember.HashHexString] = newMember.NodeIndex
			return nil
		}
		return nil
	}
	// node's directory list is complete
	for hashHexString, nodeIndex := range els.directoryCommittee {
		if nodeIndex == els.index {
			directoryMember := els.members[hashHexString]
			go func(directoryMember *Member) {directoryMember.memberToDirectoryChan <- newMember}(directoryMember)
			// directory committee nodes also have to know their committee
			for hashHexString, nodeIndex := range els.directoryCommittee {
				go func(directoryMember *Member, hashHexString string, nodeIndex int) {
					directoryMember.memberToDirectoryChan <- &NewMember{
						hashHexString , nodeIndex}
				}(directoryMember, hashHexString, nodeIndex)
			}
		}
	}
	return nil
}


func (els *Elastico) runAsDirectory (directoryMember *Member) {
	log.Lvl2("member", directoryMember.hashHexString[0:8], "is directory on node", els.index )
	for {
		newMember := <- directoryMember.memberToDirectoryChan
		hashByte, _ := hex.DecodeString(newMember.HashHexString)
		committeeNo := els.getCommitteeNo(hashByte)
		if len(directoryMember.directory[committeeNo]) < els.CommitteeSize {
			//FIXME add some validation before accepting the id
			committeeMap := directoryMember.directory[committeeNo]
			committeeMap[newMember.HashHexString] = newMember.NodeIndex
		}
		completeCommittee := 0
		for i := 0 ; i < els.CommitteeCount; i++ {
			if len(directoryMember.directory[i]) >= els.CommitteeSize {
				completeCommittee++
			}
		}
		if completeCommittee == els.CommitteeCount {
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
		if hashInt.Bit(i) == 1 {
			toReturn += 1 << uint(i)
		}
	}
	return toReturn
}


func (els *Elastico) multicast(directoryMember *Member) error {
	log.Lvl2("directory member", directoryMember.hashHexString[0:8], "multicast on node", els.index)
	finalCommittee := els.finalCommittee
	for committee, _:= range directoryMember.directory{
		for member, node := range directoryMember.directory[committee]{
			if err := els.SendTo(els.nodeList[node],
				&CommitteeMembers{
					directoryMember.directory[committee],
					directoryMember.directory[finalCommittee] ,
					member, committee}); err != nil{
				log.Error(els.Name(), "directory failed to send committee members", err)
				return err
			}
		}
	}
	return nil
}


func (els *Elastico) handleCommitteeMembers(committee *CommitteeMembers) error {
	els.mmtx.Lock()
	memberToUpdate := els.members[committee.DestMember]
	els.mmtx.Unlock()
	return els.checkForPBFT(memberToUpdate, committee)
}

func (els *Elastico) checkForPBFT(memberToUpdate *Member, committee *CommitteeMembers) error {
	if memberToUpdate.noDirMsg {
		return nil
	}
	memberToUpdate.committeeNo = committee.CommitteeNo
	if memberToUpdate.committeeNo == -1 {
		memberToUpdate.committeeNo = committee.CommitteeNo
	}
	for coMember, node := range committee.CoMembers {
		if memberToUpdate.hashHexString == coMember{
			continue
		}
		memberToUpdate.committeeMembers[coMember] = node
		memberToUpdate.committeeMembersCnt[coMember]++
	}
	for finMember, node := range committee.FinMembers {
		if memberToUpdate.hashHexString == finMember {
			memberToUpdate.isFinal = true
			continue
		}
		memberToUpdate.finalCommitteeMembers[finMember] = node
		memberToUpdate.finalCommitteeMembersCnt[finMember]++
	}
	memberToUpdate.directoryMsgCnt++
	if memberToUpdate.directoryMsgCnt >= els.threshold {
		log.Lvl2("member", memberToUpdate.hashHexString[0:8],
			"on node", els.index, "has received comembers with",
			memberToUpdate.directoryMsgCnt, "directory messages")
		memberToUpdate.directoryMsgCnt = 0
		memberToUpdate.noDirMsg = true
		for coMember := range memberToUpdate.committeeMembers {
			if memberToUpdate.committeeMembersCnt[coMember] < els.threshold {
				delete(memberToUpdate.committeeMembers, coMember)
			}
		}
		for finMember := range memberToUpdate.finalCommitteeMembers {
			if memberToUpdate.finalCommitteeMembersCnt[finMember] < els.threshold {
				delete(memberToUpdate.finalCommitteeMembers, finMember)
			}
		}
		//els.state = stateConsensus
		leader := selectLeader(memberToUpdate.committeeMembers)
		if memberToUpdate.hashHexString < leader {
			leader = memberToUpdate.hashHexString
		}
		if memberToUpdate.hashHexString == leader {
			memberToUpdate.isLeader = true
			memberToUpdate.thresholdPBFT = int(math.Ceil(float64(len(memberToUpdate.committeeMembers))*2.0/3.0)) - 1
			go els.startPBFT(memberToUpdate)
			go els.handlePreparePBFT(memberToUpdate)
			<- memberToUpdate.startPrepareChan
		} else {
			memberToUpdate.thresholdPBFT = int(math.Ceil(float64(len(memberToUpdate.committeeMembers))*2.0/3.0))
			go els.handlePrePreparePBFT(memberToUpdate)
			<- memberToUpdate.startPrePrepareChan
		}
	}
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
	log.Lvl2("member", member.hashHexString[0:8] , "of committee", member.committeeNo,
		"on node", els.index, "has started pbft")
	log.Lvl2(member.committeeMembers, "for member", member.hashHexString[0:8], "on node", els.index)
	for coMember, node := range member.committeeMembers{
		if err := els.SendTo(els.nodeList[node],
			&PrePrepare{member.committeeBlock, coMember}); err != nil{
			log.Error(els.Name(), "could not start PBFT", els.nodeList[node], err)
			return
		}
	}
}


func (els *Elastico) handlePrePrepare(prePrepare *PrePrepare) error {
	member := els.members[prePrepare.DestMember]
	log.Lvl3(member.committeeMembers, "for member", member.hashHexString[0:8], "on node", els.index)
	if member.state != pbftStatePrePrepare {
		// FIXME make this channel with buffer
		member.prePrepareChan <- prePrepare
		return nil
	}
	member.prePrepareChan <- prePrepare
	return nil
}


func (els *Elastico) handlePrePreparePBFT(member *Member) {
	log.Lvl2("member", member.hashHexString[0:8],"of committee", member.committeeNo,
		"on node", els.index, "is on handle preprepare")
	member.state = pbftStatePrePrepare
	member.startPrePrepareChan <- true
	block := <- member.prePrepareChan
	member.committeeBlock = block.TrBlock
	go els.handlePreparePBFT(member)
	<- member.startPrepareChan
	for coMember, node := range member.committeeMembers {
		if err := els.SendTo(els.nodeList[node],
			&Prepare{member.committeeBlock.HeaderHash, coMember}); err != nil {
			log.Error(els.Name(), "can't send prepare message", els.nodeList[node].Name(), err)
		}
	}
	go func(){
		for _, prepare := range member.tempPrepareMsg {
			member.prepareChan <- prepare
		}
		member.tempPrepareMsg = nil
	}()
	return
}


func (els *Elastico) handlePrepare(prepare *Prepare) error {
	member := els.members[prepare.DestMember]
	if member.state != pbftStatePrepare {
		member.tempPrepareMsg = append(member.tempPrepareMsg, prepare)
		return nil
	}
	member.prepareChan <- prepare
	return nil
}


func (els *Elastico) handlePreparePBFT(member *Member) {
	log.Lvl2("member", member.hashHexString[0:8], "of committee", member.committeeNo,
		"on node", els.index, "is on handle prepare")
	member.state = pbftStatePrepare
	member.startPrepareChan <- true
	for {
		<- member.prepareChan
		member.prepMsgCount++
		if member.prepMsgCount >= member.thresholdPBFT {
			member.prepMsgCount = 0
			go els.handleCommitPBFT(member)
			<- member.startCommitChan
			for coMember, node := range member.committeeMembers {
				if err := els.SendTo(els.nodeList[node],
					&Commit{member.committeeBlock.HeaderHash, coMember}); err != nil {
					log.Error(els.Name(), "can't send prepare message", els.nodeList[node].Name(), err)
				}
			}
			go func(){
				for _, commit := range member.tempCommitMsg {
					member.commitChan <- commit
				}
				member.tempCommitMsg = nil
			}()
			return
		}
	}
}

func (els *Elastico) handleCommit(commit *Commit) error{
	member := els.members[commit.DestMember]
	if member.state != pbftStateCommit {
		member.tempCommitMsg = append(member.tempCommitMsg, commit)
		return nil
	}
	member.commitChan <- commit
	return nil
}


func (els *Elastico) handleCommitPBFT(member *Member) {
	log.Lvl2("member", member.hashHexString[0:8], "of committee", member.committeeNo,
		"on node", els.index, "is on handle commit for Block", member.committeeBlock.HeaderHash[0:8])
	member.state = pbftStateCommit
	member.startCommitChan <- true
	for {
		<- member.commitChan
		member.commitMsgCount++
		if member.commitMsgCount >= member.thresholdPBFT {
			member.commitMsgCount = 0
			if member.isFinal{
				log.Lvl2("member", member.hashHexString[0:8], "of committee", member.committeeNo, "on node",
				els.index, "is final member")
				go els.finalPBFT(member)
				<- member.startFinalPBFTChan
				for _, block := range member.tempBlockToFinalCommittee{
					member.finalBlockChan <- block
				}
			} else {
				els.broadcastToFinal(member)
			}
			return
		}
	}
}


func (els *Elastico) broadcastToFinal (member *Member) {
	log.Lvl2("member", member.hashHexString[0:8], "of committee", member.committeeNo,
		"on node", els.index, "broadcasts to final committee")
	for finMember, node := range member.finalCommitteeMembers {
		if err := els.SendTo(els.nodeList[node],
			&BlockToFinalCommittee{member.committeeBlock.HeaderHash,
								   finMember, member.committeeNo}); err != nil {
			log.Error(els.Name(), "can't send Block to final committee", els.nodeList[node], err)
			return
		}
	}
	member.state = pbftStateFinish
	return
}


func (els *Elastico) handleBlockToFinalCommittee(block *BlockToFinalCommittee) {
	member := els.members[block.DestMember]
	if member.state != pbftStateTransit {
		member.tempBlockToFinalCommittee = append(member.tempBlockToFinalCommittee, block)
		return
	}
	go func(block *BlockToFinalCommittee){
		member.finalBlockChan <- block
	}(block)
}


func (els *Elastico) finalPBFT(member *Member) {
	// FIXME maybe fix this if we have other problems with races on other calls to pbft phases
	member.startFinalPBFTChan <- true
	member.state = pbftStateTransit
	log.Lvl3("member", member.hashHexString[0:8], "of committee", member.committeeNo, "on node", els.index, "is busy with final pbft")
	for {
		block := <- member.finalBlockChan
		member.finalConsensus[member.committeeNo] = member.committeeBlock.HeaderHash
		member.finalConsensus[block.CommitteeNo] = block.HeaderHash
		log.Lvl2("member", member.hashHexString[0:8], "has received", len(member.finalConsensus), "blocks")
		if len(member.finalConsensus) == els.CommitteeCount {
			if member.isLeader {
				// FiXME print stuff
				log.Lvl2("member", member.hashHexString[0:8], "of committee", member.committeeNo,
				"on node", els.index, "has started final pbft")
				go els.handlePrepareFinalPBFT(member)
				<- member.startPrepareFinalChan
				for coMember, node := range member.committeeMembers{
					if err := els.SendTo(els.nodeList[node],
						&PrePrepareFinal{member.finalConsensus[0], coMember}); err != nil{
						log.Error(els.Name(), "can't start final consensus", els.nodeList[node], err)
					}
				}
			} else {
				go els.handlePrePrepareFinalPBFT(member)
				<- member.startPrePrepareFinalChan
			}
			return
		}
	}
}

func (els *Elastico) handlePrePrepareFinal(prePrepareFinal *PrePrepareFinal){
	member := els.members[prePrepareFinal.DestMember]
	if member.state != pbftStatePrePrepareFinal {
		member.prePrepareFinalChan <- prePrepareFinal
		return
	}
	member.prePrepareFinalChan <- prePrepareFinal
	return
}

func (els *Elastico) handlePrePrepareFinalPBFT(member *Member) {
	// FIXME print stuff
	log.Lvl2("member", member.hashHexString[0:8], "of committee", member.committeeNo,
	"on node", els.index, "is on handle preprepare final")
	member.state = pbftStatePrePrepareFinal
	member.startPrePrepareFinalChan <- true
	block := <- member.prePrepareFinalChan
	if block == nil {
		return
	}
	go els.handlePrepareFinalPBFT(member)
	<- member.startPrepareFinalChan
	for coMember, node := range member.committeeMembers{
		if err := els.SendTo(els.nodeList[node],
			&PrepareFinal{block.HeaderHash, coMember}); err != nil{
			log.Error(els.Name(), "can't send preprepare in final pbft", els.nodeList[node], err)
		}
	}
	go func(){
		for _, prepare := range member.tempPrepareFinalMsg {
			member.prepareFinalChan <- prepare
		}
		member.tempPrepareFinalMsg = nil
	}()
	return
}

func (els *Elastico) handlePrepareFinal(prepareFinal *PrepareFinal) {
	member := els.members[prepareFinal.DestMember]
	if member.state != pbftStatePrepareFinal {
		member.tempPrepareFinalMsg = append(member.tempPrepareFinalMsg, prepareFinal)
		return
	}
	member.prepareFinalChan <- prepareFinal
	return
}

func (els *Elastico) handlePrepareFinalPBFT(member *Member){
	log.Lvl2("member", member.hashHexString[0:8], "of committee", member.committeeNo,
		"on node", els.index, "is on handle prepare final")
	member.state = pbftStatePrepareFinal
	member.startPrepareFinalChan <- true
	for {
		block := <- member.prepareFinalChan
		member.prepFinalMsgCount++
		if member.prepFinalMsgCount >= member.thresholdPBFT {
			member.prepFinalMsgCount = 0
			go els.handleCommitFinalPBFT(member)
			<- member.startCommitFinalChan
			for coMember, node := range member.committeeMembers {
				if err := els.SendTo(els.nodeList[node],
					&CommitFinal{block.HedearHash, coMember}); err != nil{
					log.Error(els.Name(), "can't send to member in final commit", els.nodeList[node], err)
				}
			}
			go func(){
				for _, commit := range member.tempCommitFinalMsg {
					member.commitFinalChan <- commit
				}
				member.tempCommitFinalMsg = nil
			}()
			return
		}
	}
}

func (els *Elastico) handleCommitFinal(commitFinal *CommitFinal) {
	member := els.members[commitFinal.DestMember]
	if member.state != pbftStateCommitFinal {
		member.tempCommitFinalMsg = append(member.tempCommitFinalMsg, commitFinal)
		return
	}
	member.commitFinalChan <- commitFinal
	return
}

func (els *Elastico) handleCommitFinalPBFT(member *Member) {
	log.Lvl2("member", member.hashHexString[0:8], "of committee", member.committeeNo,
		"on node", els.index, "is on handle commit final")
	member.state = pbftStateCommitFinal
	member.startCommitFinalChan <- true
	for {
		<- member.commitFinalChan
		member.commitFinalMsgCount++
		if member.commitFinalMsgCount >= member.thresholdPBFT {
			member.commitFinalMsgCount = 0
			member.state = pbftStateFinish
			els.broadcast(func(tn *onet.TreeNode){
				if err := els.SendTo(tn, &Finish{}); err != nil {
					log.Error("can't send finish message")
				}
			})
			return
		}
	}
}


func (els *Elastico) handleFinish() {
	els.finishMsgCnt++
	if els.finishMsgCnt >= int(math.Ceil(float64(els.CommitteeSize)*2.0/3.0)){
		els.finishMsgCnt = 0
		els.SendTo(els.Tree().Root, &FinishRoot{})
		if !els.IsRoot(){
			els.Done()
		}
	}
}


func (els *Elastico) addMember (hashHexString string) *Member{
	member := new(Member)
	member.hashHexString = hashHexString
	member.state = pbftStateNotReady
	member.committeeBlock = els.block
	member.committeeNo = -1
	els.mmtx.Lock()
	els.members[hashHexString] = member
	els.mmtx.Unlock()
	member.directory = make(map[int](map[string]int))
	for i := 0; i < els.CommitteeCount; i++{
		member.directory[i] = make(map[string]int)
	}
	member.committeeMembers = make(map[string]int)
	member.committeeMembersCnt = make(map[string]int)
	member.finalCommitteeMembers = make(map[string]int)
	member.finalCommitteeMembersCnt = make(map[string]int)
	member.finalConsensus = make(map[int]string)
	member.memberToDirectoryChan = make(chan *NewMember, els.CommitteeSize)
	member.prePrepareChan = make(chan *PrePrepare, els.CommitteeSize)
	member.startPrePrepareChan = make(chan bool)
	member.prepareChan = make(chan *Prepare, els.CommitteeSize)
	member.startPrepareChan = make(chan bool)
	member.commitChan = make(chan *Commit, els.CommitteeSize)
	member.startCommitChan = make(chan bool)
	member.prePrepareFinalChan = make(chan *PrePrepareFinal, els.CommitteeSize)
	member.startPrePrepareFinalChan = make(chan bool)
	member.prepareFinalChan = make(chan *PrepareFinal, els.CommitteeSize)
	member.startPrepareFinalChan = make(chan bool)
	member.commitFinalChan = make(chan *CommitFinal, els.CommitteeSize)
	member.startCommitFinalChan = make(chan bool)
	member.startFinalPBFTChan = make(chan bool)
	member.finalBlockChan = make(chan *BlockToFinalCommittee, els.CommitteeSize)
	return member
}

func (els *Elastico) broadcast(sendCB func(node *onet.TreeNode)){
	for _ ,node := range els.nodeList{
		go sendCB(node)
	}
}

func (els *Elastico) broadcastToDirectory(sendCB func(node *onet.TreeNode)) {
	for _, node := range els.directoryCommittee{
		go sendCB(els.nodeList[node])
	}
}
