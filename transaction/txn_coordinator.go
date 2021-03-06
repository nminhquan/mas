package transaction

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/nminhquan/mas/config"

	"github.com/nminhquan/mas/client"
	"github.com/nminhquan/mas/credentials"
	"github.com/nminhquan/mas/dao"
	"github.com/nminhquan/mas/db"
	"github.com/nminhquan/mas/model"
	pb "github.com/nminhquan/mas/proto"
	"github.com/nminhquan/mas/utils"

	"google.golang.org/grpc"
	grpc_creds "google.golang.org/grpc/credentials"
)

var (
	txnDao dao.TxnCoordinatorDAO
	creds  = func() grpc_creds.TransportCredentials {
		c, err := grpc_creds.NewServerTLSFromFile(credentials.SSL_SERVER_CERT, credentials.SSL_SERVER_PRIVATE_KEY)
		if err != nil {
			log.Fatalf("Cannot get credentials: %v", err)
		}
		return c
	}()
	maxId    int64 = 0
	peerList map[string]string
	// resultBlockQueue, _ = blockingQueues.NewArrayBlockingQueue(uint64(100000000))
)

func init() {
	log.Println("TC init")
	cacheService := db.NewCacheService(config.RedisHost, "")
	lockDB, _ := db.NewRocksDB(fmt.Sprintf("%v/%v", config.RocksDBDir, "meta_db"))
	log.Println("lockDB: ", lockDB)
	txnDao = dao.NewTxnCoordinatorDAO(cacheService, lockDB)
	peerList = txnDao.GetPeersList()
}

func RefreshPeerList() {
	peerList = txnDao.GetPeersList()
	log.Println("RefreshPeerList: ", peerList)
}

type TxnCoordinator struct {
	port string
	mtx  sync.Mutex
}

func CreateTCServer(port string) *TxnCoordinator {

	return &TxnCoordinator{port, sync.Mutex{}}
}

func (tc *TxnCoordinator) GetAccount(ctx context.Context, in *pb.AccountRequest) (*pb.AccountReply, error) {
	peerBucket := txnDao.GetPeerBucket(in.AccountNumber)
	if peerBucket == "" {
		message := fmt.Sprintln("Account doesn't exist")
		return &pb.AccountReply{Message: message}, nil
	}
	rmClient := client.CreateRMClient(strings.Split(peerBucket, ","))

	accInfo := &model.AccountInfo{Number: in.AccountNumber}
	instruction := model.Instruction{Type: model.INS_TYPE_QUERY_ACCOUNT, Data: accInfo}
	message, err := rmClient.CreateGetAccountRequest(instruction)

	return &pb.AccountReply{Message: message}, err
}

func (tc *TxnCoordinator) CreateAccount(ctx context.Context, in *pb.AccountRequest) (*pb.AccountReply, error) {
	accInfo := &model.AccountInfo{Number: in.AccountNumber, Balance: in.Balance}
	peers := assignPeers()
	rmClient := client.CreateRMClient(peers)
	globalLock := client.CreateLockClient(config.LockServHost, accInfo.Number)
	globalTxnId := utils.GenXid()
	localTxnId := utils.GenXid()
	instruction := model.Instruction{Type: model.INS_TYPE_CREATE_ACCOUNT, Data: accInfo, XID: globalTxnId}
	var localTxn = NewLocalTransaction(rmClient, globalLock, instruction, localTxnId, globalTxnId)
	subTxns := []Transaction{
		localTxn,
	}
	var txn Transaction = NewGlobalTransaction(subTxns, globalTxnId)
	var message string
	if txn.Prepare() {
		if bucket := txnDao.GetPeerBucket(in.AccountNumber); bucket != "" {
			message := fmt.Sprintln("FAIL: Account already exists, id = ", in.AccountNumber, " bucket = ", bucket)
			log.Println(message)
			return &pb.AccountReply{Message: message}, nil
		}
	} else {
		message = model.RPC_MESSAGE_FAIL + " Cannot Prepare() global transaction"
		return &pb.AccountReply{Message: message}, nil
	}

	if txn.Begin() {
		txnDao.CreateTransactionEntry(globalTxnId, utils.GetCurrentTimeInMillis(), model.TXN_STATE_COMMITTED, fmt.Sprintf("%v", accInfo.Number))
		txn.Commit()
		txnDao.InsertPeerBucket(accInfo.Number, strings.Join(peers, ","))
		message = model.RPC_MESSAGE_OK
	} else {
		txnDao.CreateTransactionEntry(globalTxnId, utils.GetCurrentTimeInMillis(), model.TXN_STATE_ABORTED, fmt.Sprintf("%v", accInfo.Number))
		message = model.RPC_MESSAGE_FAIL
	}
	return &pb.AccountReply{Message: message}, nil
}

func (tc *TxnCoordinator) CreatePayment(ctx context.Context, in *pb.PaymentRequest) (*pb.PaymentReply, error) {
	// check MetaDB
	fromPeerBucket := txnDao.GetPeerBucket(in.FromAccountNumber)
	toPeerBucket := txnDao.GetPeerBucket(in.ToAccountNumber)

	paymentValid := tc.validatePayment(in.FromAccountNumber, in.ToAccountNumber, fromPeerBucket, toPeerBucket)

	if !paymentValid {
		message := "FAIL: Invalid payment information, please check again"
		log.Println(message)
		return &pb.PaymentReply{Message: message}, nil
	}

	rmClientFrom := client.CreateRMClient(strings.Split(fromPeerBucket, ","))
	rmClientTo := client.CreateRMClient(strings.Split(toPeerBucket, ","))
	log.Printf("rmClientFrom: %v, rmClientTo: %v", rmClientFrom, rmClientTo)
	pmInfo := model.PaymentInfo{From: in.FromAccountNumber, To: in.ToAccountNumber, Amount: in.Amount}
	globalLockFrom := client.CreateLockClient(config.LockServHost, pmInfo.From)
	globalLockTo := client.CreateLockClient(config.LockServHost, pmInfo.To)

	globalTxnId := utils.GenXid()
	instructionFrom := model.Instruction{Type: model.INS_TYPE_SEND_PAYMENT, Data: pmInfo, XID: globalTxnId}
	instructionTo := model.Instruction{Type: model.INS_TYPE_RECEIVE_PAYMENT, Data: pmInfo, XID: globalTxnId}
	var localTxnFrom = NewLocalTransaction(rmClientFrom, globalLockFrom, instructionFrom, utils.GenXid(), globalTxnId)
	var localTxnTo = NewLocalTransaction(rmClientTo, globalLockTo, instructionTo, utils.GenXid(), globalTxnId)
	subTxns := []Transaction{
		localTxnFrom,
		localTxnTo,
	}
	var txn Transaction = NewGlobalTransaction(subTxns, globalTxnId)
	currentTs := utils.GetCurrentTimeInMillis()

	var message string
	if txn.Prepare() {

	} else {
		message = model.RPC_MESSAGE_FAIL + " Cannot Prepare() global transaction"
		return &pb.PaymentReply{Message: message}, nil
	}
	if txn.Begin() {
		log.Printf("[GlobalTXN:%v] Commit", globalTxnId)
		txn.Commit()
		txnDao.CreateTransactionEntry(globalTxnId, currentTs, model.TXN_STATE_COMMITTED, fmt.Sprintf("%v,%v", pmInfo.From, pmInfo.To))
		message = model.TXN_STATE_COMMITTED
	} else {
		log.Printf("[GlobalTXN:%v] Rollback", globalTxnId)
		txnDao.CreateTransactionEntry(globalTxnId, currentTs, model.TXN_STATE_ABORTED, fmt.Sprintf("%v,%v", pmInfo.From, pmInfo.To))
		message = model.TXN_STATE_ABORTED
		txn.Rollback()
	}

	return &pb.PaymentReply{Message: message}, nil
}

func (tc *TxnCoordinator) validatePayment(from string, to string, fromPeerBucket string, toPeerBucket string) bool {
	if from == to {
		log.Println("Cannot send money to yourself.")
		return false
	} else if fromPeerBucket == "" {
		log.Println("From account: ", from, " doesn't exist")
		return false
	} else if toPeerBucket == "" {
		log.Println("To account: ", from, " doesn't exist")
		return false
	}

	return true
}

func (tc *TxnCoordinator) Start() {
	lis, err := net.Listen("tcp", tc.port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	// register JWTServerInterceptor for authentication

	s := grpc.NewServer(grpc.UnaryInterceptor(credentials.JWTServerInterceptor), grpc.Creds(creds))
	pb.RegisterAccountServiceServer(s, tc)
	log.Printf("[TxnCoordinator] RPC server started at port: %s", tc.port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}

}

func (tc *TxnCoordinator) TestMethod(ctx context.Context, in *pb.TestMessage) (*pb.TestMessage, error) {
	return &pb.TestMessage{Message: "pong"}, nil
}
