package elastico

import (
"github.com/dedis/cothority/byzcoin/blockchain"
"gopkg.in/dedis/onet.v1"
)

const (

	preprepare = iota
	prepare
	commit
	finish
)

type PrePrepare struct {
	*blockchain.TrBlock
	HeaderHash string
}

type prePrepareChan struct {
	*onet.TreeNode
	PrePrepare
}

// Prepare is the prepare packet
type Prepare struct {
	HeaderHash string
}

type prepareChan struct {
	*onet.TreeNode
	Prepare
}

// Commit is the commit packet in the protocol
type Commit struct {
	HeaderHash string
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

type groupCommitteeIDChan struct {
	*onet.TreeNode
	Committee
}

type directoryCommittee struct{
	id string
	nodeIndex int
}

type directoryCommitteeChan struct{
	*onet.TreeNode
	directoryCommittee
}

type finalCommitteeChan struct{
	*onet.TreeNode
	Committee
}

type id struct {
	id string
	nodeIndex int
}

type idChan struct{
	*onet.TreeNode
	id
}
type Committee struct{
	Indices []int
	Index int
	ID string
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

type ResendID struct {
	Resend bool
}

type resendIDChan struct{
	*onet.TreeNode
	ResendID
}

type startProtocolChan struct{
	*onet.TreeNode
	startProtocol
}

type startProtocol struct{
	start bool
}

