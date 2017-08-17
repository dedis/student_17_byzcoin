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

	// the target for the PoW
	target big.Int
	// the global threshold of nodes needed computed from committee members count
	threshold int


	CommitteeSize  int
	CommitteeCount int
	committeeBits  int
	finalCommittee int


	tempNewMemberMsg []NewMember
	// all the members that this node has produced
	members map[string]*Member
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
}




type Member struct {
	//id hash of this member
	hashHexString string
	// committee number
	committeeNo int
	// members in this member's committee
	committeeMembers map[string]int
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
	// map of committee to block that they reached consensus
	finalConsensus map[int]string
	prePrepareChan chan *PrePrepare
	prePrepareFinalChan chan *PrePrepareFinal
	prepareChan    chan *Prepare
	prepareFinalChan chan *PrepareFinal
	commitChan     chan *Commit
	commitFinalChan chan *CommitFinal
	finalBlockChan chan *BlockToFinalCommittee
}



func NewElastico(n *onet.TreeNodeInstance) (*Elastico, error){
	els := new(Elastico)
	els.TreeNodeInstance = n
	els.nodeList = n.Tree().List()
	els.index = -1
	els.threshold = int (math.Ceil(float64(els.CommitteeSize) * 2.0 / 3.0))
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
	return els, nil
}


func (els *Elastico) Start() error {
	// FIXME print stuff
	log.Print("root has started protocol")
	// broadcast to all nodes so that they start the protocol
	els.broadcast(func (tn *onet.TreeNode){
		if err := els.SendTo(tn,
			&StartProtocol{els.RootNodeBlocks[rand.Intn(els.CommitteeCount)],
				els.CommitteeCount,
				els.CommitteeSize}); err != nil {
			log.Error(els.Name(), "can't start protocol", tn.Name(), err)
			return err
		}
	})
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
				els.rootFinishMsgCnt++
				if els.rootFinishMsgCnt == els.Tree().Size(){
					els.OnDoneCB()
				}
		}
	}
}


func (els *Elastico) handleStartProtocol(start *StartProtocol) error {
	els.CommitteeCount = start.committeeCount
	els.CommitteeSize = start.committeeSize
	els.committeeBits = int (math.Log2(float64(els.CommitteeCount)))
	els.finalCommittee = rand.Intn(els.CommitteeCount)
	els.block = start.block
	els.state = stateMining
	// FIXME print stuff
	log.Print("node", els.index, "has started")
	go els.findPoW()
	return nil
}

func (els *Elastico) findPoW() {
	for i := 0; true; i++ {
		if els.state == stateMining {
			h := sha256.New()
			h.Write([]byte(string(els.index + i)))
			hashByte := h.Sum(nil)
			hashInt := new(big.Int).SetBytes(hashByte)
			if hashInt.Cmp(els.target) == -1 {
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
	log.Print("node", els.index, "has found", hashHexString)
	if len(els.directoryCommittee) < els.CommitteeSize {
		member := els.addMember(hashHexString)
		els.directoryCommittee[hashHexString] = els.index
		go els.runAsDirectory(member)
		els.broadcast(func(tn *onet.TreeNode) {
			if err := els.SendTo(tn,
				&NewMember{hashHexString, els.index}); err != nil {
				log.Error(els.Name(), "can't broadcast new member as directory", tn.Name(), err)
				return err
			}
		})
	} else {
		els.addMember(hashHexString)
		els.broadcastToDirectory(func(tn *onet.TreeNode) {
			if err := els.SendTo(tn,
				&NewMember{hashHexString, els.index}); err != nil {
				log.Error(els.Name(), "can't broadcast new member to directory", tn.Name(), err)
				return err
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
	els.dc.Unlock()
	if len(els.directoryCommittee) < els.CommitteeSize {
		if els.index != newMember.NodeIndex {
			els.directoryCommittee[newMember.HashHexString] = newMember.NodeIndex
			return
		}
		return
	}
	// node's directory list is complete
	for hashHexString, nodeIndex := range els.directoryCommittee {
		if nodeIndex == els.index {
			directoryMember := els.members[hashHexString]
			go func(directoryMember *Member) {directoryMember.memberToDirectoryChan <- newMember}(directoryMember)
			// directory committee nodes also has to know their committee
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
		if hashInt.Bit(i) {
			toReturn += 1 << uint(i)
		}
	}
	return toReturn
}


func (els *Elastico) multicast(directoryMember *Member) error {
	finalCommittee := rand.Intn(els.CommitteeCount)
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


func (els *Elastico) handleCommitteeMembers(committee CommitteeMembers) error {
	memberToUpdate := els.members[committee.DestMember]
	return els.checkForPBFT(memberToUpdate, committee)
}

func (els *Elastico) checkForPBFT(memberToUpdate *Member, committee CommitteeMembers) error {
	if els.state == stateMining {
		for coMember, node := range committee.CoMembers {
			if memberToUpdate.hashHexString == coMember{
				continue
			}
			memberToUpdate.committeeMembers[coMember] = node
		}
		for finMember, node := range committee.FinMembers {
			if memberToUpdate.hashHexString == finMember {
				memberToUpdate.isFinal = true
				continue
			}
			memberToUpdate.finalCommitteeMembers[finMember] = node
		}
		memberToUpdate.directoryMsgCnt++
		if memberToUpdate.directoryMsgCnt >= els.threshold {
			memberToUpdate.directoryMsgCnt = 0
			els.state = stateConsensus
			leader := selectLeader(memberToUpdate.committeeMembers)
			if leader == memberToUpdate.hashHexString {
				memberToUpdate.isLeader = true
				go els.startPBFT(memberToUpdate)
			}
			close(memberToUpdate.isLeaderChan)
			if memberToUpdate.isLeader {
				memberToUpdate.thresholdPBFT = int(math.Ceil(float64(len(memberToUpdate.committeeMembers))*2.0/3.0)) - 1
				memberToUpdate.state = pbftStatePrepare
			} else {
				memberToUpdate.thresholdPBFT = int(math.Ceil(float64(len(memberToUpdate.committeeMembers))*2.0/3.0))
				memberToUpdate.state = pbftStatePrePrepare
				go els.handlePrePreparePBFT(memberToUpdate)
			}
		}
		return nil
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
	if member.state != pbftStatePrePrepare {
		// FIXME make this channel with buffer
		member.prePrepareChan <- prePrepare
		return nil
	}
	member.prePrepareChan <- prePrepare
	return nil
}


func (els *Elastico) handlePrePreparePBFT(member *Member) {
	block := <- member.prePrepareChan
	member.state = pbftStatePrepare
	member.committeeBlock = block.TrBlock
	go els.handlePreparePBFT(member)
	for coMember, node := range member.committeeMembers {
		if err := els.SendTo(els.nodeList[node],
			&Prepare{member.committeeBlock.HeaderHash, coMember}); err != nil {
			log.Error(els.Name(), "can't send prepare message", els.nodeList[node].Name(), err)
		}
	}
	for _, prepare := range member.tempPrepareMsg {
		member.prepareChan <- prepare
	}
	member.tempPrepareMsg = nil
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
	for {
		<- member.prepareChan
		member.prepMsgCount++
		if member.prepMsgCount >= member.thresholdPBFT {
			member.prepMsgCount = 0
			member.state = pbftStateCommit
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
	member := els.members[commit.DestMember]
	if member.state != pbftStateCommit {
		member.tempCommitMsg = append(member.tempCommitMsg, commit)
		return nil
	}
	member.commitChan <- commit
	return nil
}


func (els *Elastico) handleCommitPBFT(member *Member) {
	for {
		<- member.commitChan
		member.commitMsgCount++
		if member.commitMsgCount >= member.thresholdPBFT {
			member.commitMsgCount = 0
			if member.isFinal{
				member.state = pbftStatePrePrepareFinal
				go els.handlePrePrepareFinalPBFT(member)
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
	for finMember, node := range member.finalCommitteeMembers {
		if err := els.SendTo(els.nodeList[node],
			&BlockToFinalCommittee{member.committeeBlock.HeaderHash,
								   finMember, member.committeeNo}); err != nil {
			log.Error(els.Name(), "can't send block to final committee", els.nodeList[node], err)
			return
		}
	}
	member.state = pbftStateFinish
	go els.checkFinish()
}


func (els *Elastico) handleBlockToFinalCommittee(block *BlockToFinalCommittee) {
	member := els.members[block.DestMember]
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
		if len(member.finalConsensus) == els.CommitteeCount {
			if member.isLeader {
				member.state = pbftStatePrepareFinal
				go els.handlePrepareFinalPBFT(member)
				close(member.prePrepareFinalChan)
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
	member := els.members[prePrepareFinal.DestMember]
	if member.state != pbftStatePrePrepareFinal {
		member.prePrepareFinalChan <- prePrepareFinal
		return
	}
	member.prePrepareFinalChan <- prePrepareFinal
	return
}

func (els *Elastico) handlePrePrepareFinalPBFT(member *Member) {
	block := <- member.prePrepareFinalChan
	if block == nil {
		return
	}
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
	member := els.members[prepareFinal.DestMember]
	if member.state != pbftStatePrepareFinal {
		member.tempPrepareFinalMsg = append(member.tempPrepareFinalMsg, &prepareFinal)
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
	member := els.members[commitFinal.DestMember]
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
		for _, member := range els.members {
			if member.state == pbftStateNotReady {
				pointlessMembers++
			}
		}
		nodeMembersCnt := len(els.members) - pointlessMembers
		if els.finishMsgCnt == nodeMembersCnt {
			els.SendTo(els.Tree().Root, &Finish{})
			els.Done()
			return
		}
		els.fmc.Unlock()
	}

}

func (els *Elastico) addMember (hashHexString string) *Member{
	member := new(Member)
	member.hashHexString = hashHexString
	member.state = pbftStateNotReady
	member.committeeBlock = els.block
	els.members[hashHexString] = member
	member.isLeaderChan = make(chan bool, 1)
	member.memberToDirectoryChan = make(chan *NewMember, 1)
	member.prePrepareChan = make(chan *PrePrepare, 1)
	member.prepareChan = make(chan *Prepare, 1)
	member.commitChan = make(chan *Commit, 1)
	member.prePrepareFinalChan = make(chan *PrePrepareFinal, 1)
	member.prepareFinalChan = make(chan *PrepareFinal, 1)
	member.commitFinalChan = make(chan *CommitFinal, 1)
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