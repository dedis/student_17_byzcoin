package elastico


import (
	"gopkg.in/dedis/onet.v1"
	"github.com/dedis/cothority/byzcoin/blockchain"
)



type Elastico struct{

	// the node we represent in
	*onet.TreeNodeInstance
	// block that the group must agree on
	groupBlock blockchain.TrBlock
	// block headers that final committee agrees on
	block string


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


	// if the node should resend its ID
	resendID chan bool
	// channel to announce your ID
	idChan chan idChan
	// channel to send group committee members to the node
	groupCommitteeChan chan groupCommitteeChan
	// channel to send directory committee members
	directoryCommitteeChan chan directoryCommitteeChan
	// channel to send final committee members
	finalCommitteeChan chan finalCommitteeChan
	// channel to receive the block in the group
	groupBlockChan chan groupBlockChan
	// channel to recieve block in group committee
	blockChan chan blockChan
	// channel to receive random string
	randomStringChan chan randomStringChan


	// if the is the leader in the committee
	isLeader bool
	// number of prepare messages received
	prepMessageCount int
	// number of commit messages received
	commitMessageCount int
	// threshold which is 2.0/3.0 in pbft
	threshold int
	// state that the node is in
	state int
	// the random string to generate
	randomString string
	// the random set to offer to next round
	randomSet []string

	// packages and arrays just ctrl+c ^ ctrl+v from pbft package in dedis/byzcoin
	tempPrepareMsg []Prepare
	tempCommitMsg  []Commit

	prePrepareChan chan prePrepareChan
	prepareChan    chan prepareChan
	commitChan     chan commitChan
	finishChan         chan finishChan

}