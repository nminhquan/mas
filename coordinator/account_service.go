package coordinator

import (
	"bytes"
	"encoding/gob"
	"log"
	"mas/dao"
	"mas/db"
	"mas/model"
	"mas/utils"
	"sync"
	"time"

	"go.etcd.io/etcd/etcdserver/api/snap"
)

const DEFAULT_BALANCE float64 = 0.0

type ConsensusService interface {
}

var accDao dao.AccountDAO
var pmDao dao.PaymentDAO

type AccountService interface {
	Start()
	CreateAccount(string, float64) string
	ProcessPayment(string, string, float64) string
	GetAccount(string) *model.AccountInfo
	Propose(interface{})                      //propose to RaftNode
	ReadCommits(<-chan *string, <-chan error) //read commits from RaftNode
}

type AccountServiceImpl struct {
	accDb       *db.AccountDB
	commitC     <-chan *string
	proposeC    chan<- string
	mu          sync.RWMutex
	snapshotter *snap.Snapshotter
	resultC     chan interface{}
	errorC      <-chan error
}

func (accServ *AccountServiceImpl) Start() {
	log.Printf("AccountService::Waiting for commits")
	gob.Register(model.AccountInfo{})
	gob.Register(model.PaymentInfo{})
	accServ.ReadCommits(accServ.commitC, accServ.errorC)
	go accServ.ReadCommits(accServ.commitC, accServ.errorC)
}

func CreateAccountService(accDb *db.AccountDB, commitC <-chan *string, proposeC chan<- string, snapshotter *snap.Snapshotter, errorC <-chan error) *AccountServiceImpl {
	resultC := make(chan interface{})
	accDao = dao.CreateAccountDAO(accDb)
	pmDao = dao.CreatePaymentDAO(accDb)
	return &AccountServiceImpl{accDb: accDb, commitC: commitC, proposeC: proposeC, snapshotter: snapshotter, resultC: resultC, errorC: errorC}
}

func (accServ *AccountServiceImpl) CreateAccount(accountNumber string, balance float64) string {
	// currentT := time.Now().Format(time.RFC850)
	accInfo := model.AccountInfo{
		Id:      utils.NewSHAHash(accountNumber),
		Number:  accountNumber,
		Balance: DEFAULT_BALANCE,
	}

	ins := model.Instruction{
		Type: "account",
		Data: accInfo,
	}

	accServ.Propose(ins)

	log.Printf("Waiting for result message")
	// select {
	// case accId := <-accServ.resultC:
	// 	log.Printf("Received result message %v", accId)
	// 	sAccId := accId.(string)
	// 	return sAccId
	// }
	return "account-id-fake"
}

func (accServ *AccountServiceImpl) ProcessPayment(fromAcc string, toAcc string, amount float64) string {
	// check From balance
	fromaccInfo := accServ.GetAccount(fromAcc)

	if fromaccInfo.Balance < amount {
		log.Println("not enough balance")
	}
	currentT := time.Now().Format(time.RFC850)
	pmInfo := model.PaymentInfo{
		Id:     utils.NewSHAHash(fromAcc, toAcc, currentT),
		From:   fromAcc,
		To:     toAcc,
		Amount: amount,
	}
	ins := model.Instruction{
		Type: "payment",
		Data: pmInfo,
	}
	accServ.Propose(ins)

	log.Printf("Waiting for result message")

	return "payment-id-fake"
}

func (accServ *AccountServiceImpl) GetAccount(accountNumber string) *model.AccountInfo {
	accInfo := accDao.GetAccount(accountNumber)
	return accInfo
}

//Account service as one part inside the Resource manager will manage data of its raft group, propose to its local raft node channel
// Change then will be redirected to the leader of the node and replicated to other nodes
func (accServ *AccountServiceImpl) Propose(data interface{}) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(data); err != nil {
		log.Fatal(err)
	}
	accServ.proposeC <- buf.String()
}

func (accServ *AccountServiceImpl) ReadCommits(commitC <-chan *string, errorC <-chan error) {
	log.Printf("AccountService::ReadCommits")
	for data := range commitC {
		if data == nil {
			log.Printf("AccountService::getcommitC triggered load snapshot")
			// done replaying log; new data incoming
			// OR signaled to load snapshot
			snapshot, err := accServ.snapshotter.Load()
			if err == snap.ErrNoSnapshot {
				return
			}
			if err != nil {
				log.Panic(err)
			}
			log.Printf("loading snapshot at term %d and index %d", snapshot.Metadata.Term, snapshot.Metadata.Index)
			if err := accServ.recoverFromSnapshot(snapshot.Data); err != nil {
				log.Panic(err)
			}
			continue
		}

		var dbInstruction model.Instruction
		dec := gob.NewDecoder(bytes.NewBufferString(*data))

		if err := dec.Decode(&dbInstruction); err != nil {
			log.Fatalf("raftexample: could not decode message (%v)", err)
		}
		// TODO defer func() {
		// 	if r := recover(); r != nil {
		// 		log.Println("recovered:", r)
		// 	}
		// }()
		accServ.mu.Lock()
		log.Printf("AccountService::ReadCommits Apply change to state machine %v", dbInstruction)
		// accServ.accDb.InsertAccountInfoToDB(accountData)
		accServ.ApplyInstructionToStateMachine(dbInstruction)
		accServ.mu.Unlock()

		// TODO this is the rootcause because follower pushed this message to channel but never take it
		// This should be only for leader node
		// accServ.resultC <- int64(0)
		// err := accServ.raftNode.Process(context.TODO(), raftpb.Message{})
		// log.Printf("error = %v", err)
	}
	if err, ok := <-errorC; ok {
		log.Fatal(err)
	}
}

func (accServ *AccountServiceImpl) ApplyInstructionToStateMachine(ins model.Instruction) int64 {
	log.Printf("AccountDB::ApplyInstructionToStateMachine: query = %s", ins)

	switch ins.Type {
	case "account":
		accInfo := ins.Data.(model.AccountInfo)
		accDao.CreateAccount(accInfo)
	case "payment":
		pmInfo := ins.Data.(model.PaymentInfo)
		pmDao.CreatePayment(pmInfo)
	default:
		return 0
	}

	return 1
}

// TODO func (s *kvstore) getSnapshot() ([]byte, error) {
// 	s.mu.RLock()
// 	defer s.mu.RUnlock()
// 	return json.Marshal(s.kvStore)
// }

func (s *AccountServiceImpl) recoverFromSnapshot(snapshot []byte) error {
	// var store map[string]string
	// if err := json.Unmarshal(snapshot, &store); err != nil {
	// 	return err
	// }
	// s.mu.Lock()
	// defer s.mu.Unlock()
	// s.kvStore = store
	// return nil
	panic("recoverFromSnapshot is not yet impl")
}
