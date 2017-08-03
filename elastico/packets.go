package elastico

import (
"github.com/dedis/student_17_byzcoin/byzcoin/blockchain"
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

type groupCommitteeChan struct {
	*onet.TreeNode
	Committee
}

type directoryCommitteeChan struct{
	*onet.TreeNode
	Committee
}

type finalCommitteeChan struct{
	*onet.TreeNode
	Committee
}

type idChan struct{
	*onet.TreeNode
	Committee
}
type Committee struct{
	Indices []int
	Index int
	ID string
}

type groupBlockChan struct {
	*onet.TreeNode
	GroupBlock
}

type GroupBlock struct {
	*blockchain.TrBlock
}

type blockChan struct{
	*onet.TreeNode
	Block
}

type Block struct{
	HeaderHash string
}

type randomStringChan struct{
	*onet.TreeNode
	RandomString
}

type RandomString struct{
	Random string
}

