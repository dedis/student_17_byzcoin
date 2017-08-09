package elastico

import (
"github.com/dedis/cothority/byzcoin/blockchain"
"gopkg.in/dedis/onet.v1"
)

const (
	pbftStateNotReady = iota
	pbftStatePrepare
	pbftStateCommit
	pbftStateFinish
	pbftStateFinal
	pbftStatePrepareFinal
	pbftStateCommitFinal
	pbftStateFinishFinal
)

type PrePrepare struct {
	*blockchain.TrBlock
	DestMember string
}

type PrePrepareFinal struct{
	HeaderHash string
	DestMember string
}

type prePrepareFinal struct{
	*onet.TreeNode
	PrePrepareFinal
}

type prePrepareChan struct {
	*onet.TreeNode
	PrePrepare
}

// Prepare is the prepare packet
type Prepare struct {
	HeaderHash string
	DestMember string
}

type prepareChan struct {
	*onet.TreeNode
	Prepare
}

// Commit is the commit packet in the protocol
type Commit struct {
	HeaderHash string
	DestMember string
}

type commitChan struct {
	*onet.TreeNode
	Commit
}

// Finish is just to tell the others node that the protocol is finished
type Finish struct {
	Done string
}

type finishChan struct {
	*onet.TreeNode
	Finish
}

type NewMemberID struct {
	HashHexString string
	NodeIndex     int
}

type newMemberIDChan struct{
	*onet.TreeNode
	NewMemberID
}

type committeeMembersChan struct{
	*onet.TreeNode
	CommitteeMembers
}

type CommitteeMembers struct{
	CoMembers  map[string]int
	FinMembers map[string]int
	DestMember string
}

type committeeMembersListChan struct{
	*onet.TreeNode
	CommitteeMembersList
}

type CommitteeMembersList struct {
	CoMembers map[string]int
	DestMember string
}

type finalBlockChan struct {
	*onet.TreeNode
	FinalBlock
}

type FinalBlock struct {
	*blockchain.TrBlock
}

type randomStringChan struct{
	*onet.TreeNode
	RandomString
}

type RandomString struct{
	Random string
}

type startProtocolChan struct{
	*onet.TreeNode
	StartProtocol
}

type StartProtocol struct{
	Start bool
}


type BlockToFinalCommittee struct {
	HeaderHash string
	DestMember string
	committeeNo int
}

type blockToFinalCommitteeChan struct {
	*onet.Tree
	BlockToFinalCommittee
}

type prepareFinalChan struct {
	*onet.TreeNode
	PrepareFinal
}

type PrepareFinal struct {
	HedearHash string
	DestMember string
}

