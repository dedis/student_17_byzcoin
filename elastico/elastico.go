package elastico


import (
	"gopkg.in/dedis/onet.v1"
	"github.com/dedis/cothority/byzcoin/blockchain"
	"fmt"
	"sync"
	"math"
)



type Elastico struct{
	// the node we represent in
	*onet.TreeNodeInstance
	// block that the group must agree on
	groupBlock blockchain.TrBlock
	// the id that specifies node's group
	id string
	// nodes in the tree
	nodeList []*onet.TreeNode
	// our index in the tree
	index int
	// the nodes in the same committee with this node
	groupCommittee []int
	// the nodes in the directory committee
	directoryCommittee []int
	dcm sync.Mutex
	// the nodes in the final committee
	finalCommittee []int
	// if the node is directory
	isDirecotory bool
	// map of group number to group member indices
	dir map[int]([]int)
	// number of members in each group
	memberCount int
	// if the node is final
	isFinal bool
	// the random string to generate
	randomString string
	// the random set to offer to next round
	randomSet []string
	rs sync.Mutex


	// if the node should resend its ID
	resendIDChan chan resendIDChan
	// channel to announce your ID
	idChan chan idChan
	// channel to send group committee members to the node
	groupCommitteeChan chan groupCommitteeChan
	// channel to send directory committee members
	directoryCommitteeChan chan directoryCommitteeChan
	// channel to send final committee members
	finalCommitteeChan chan finalCommitteeChan
	// channel to receive the block in the group
	finalBlockChan chan finalBlockChan
	// channel to receive random string
	randomStringChan chan randomStringChan


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
	// packages and arrays just ctrl+c ^ ctrl+v from pbft package in dedis/byzcoin
	tempPrepareMsg []Prepare
	tpm sync.Mutex
	tempCommitMsg  []Commit
	tcm sync.Mutex
	// channels for pbft
	prePrepareChan chan prePrepareChan
	prepareChan    chan prepareChan
	commitChan     chan commitChan
	finishChan     chan finishChan

}


func NewElastico(n * onet.TreeNodeInstance) (*Elastico, error){
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

	if err := n.RegisterChannel(&els.resendIDChan); err != nil{
		return els, err
	}
	if err := n.RegisterChannel(&els.idChan); err != nil {
		return els, err
	}
	if err := n.RegisterChannel(&els.groupCommitteeChan); err!= nil{
		return els, err
	}
	if err := n.RegisterChannel(&els.directoryCommitteeChan); err != nil{
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

	els.state = preprepare
	els.threshold = int(math.Ceil(float64(len(els.nodeList)) * 2.0 / 3.0))
	els.prepMsgCount = 0
	els.commitMsgCount = 0

	return els, nil
}


