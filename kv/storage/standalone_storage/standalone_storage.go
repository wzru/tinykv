package standalone_storage

import (
	"github.com/Connor1996/badger"
	"github.com/pingcap-incubator/tinykv/kv/config"
	"github.com/pingcap-incubator/tinykv/kv/storage"
	"github.com/pingcap-incubator/tinykv/kv/util/engine_util"
	"github.com/pingcap-incubator/tinykv/proto/pkg/kvrpcpb"
)

// StandAloneStorage is an implementation of `Storage` for a single-node TinyKV instance. It does not
// communicate with other nodes and all data is stored locally.
type StandAloneStorage struct {
	opt badger.Options
	db  *badger.DB
}

//NewStandAloneStorage will create a StandAloneStorage
func NewStandAloneStorage(conf *config.Config) *StandAloneStorage {
	opt := badger.DefaultOptions
	opt.Dir = conf.DBPath
	opt.ValueDir = conf.DBPath
	// db, err := badger.Open(opt)
	// if err != nil {
	// 	return nil
	// }
	return &StandAloneStorage{opt: opt}
}

//Start will start the db
func (s *StandAloneStorage) Start() error {
	var err error
	s.db, err = badger.Open(s.opt)
	return err
}

//Stop will close the db
func (s *StandAloneStorage) Stop() error {
	return s.db.Close()
}

//Reader returns a StorageReader
func (s *StandAloneStorage) Reader(ctx *kvrpcpb.Context) (storage.StorageReader, error) {
	return standAloneStorageReader{txn: s.db.NewTransaction(false)}, nil
}

//Write will write []batch to DB
func (s *StandAloneStorage) Write(ctx *kvrpcpb.Context, batch []storage.Modify) error {
	wb := engine_util.WriteBatch{}
	for _, bat := range batch {
		switch bat.Data.(type) {
		case storage.Put:
			wb.SetCF(bat.Cf(), bat.Key(), bat.Value())
		case storage.Delete:
			wb.DeleteCF(bat.Cf(), bat.Key())
		}
	}
	return wb.WriteToDB(s.db)
}

type standAloneStorageReader struct {
	txn *badger.Txn
}

func (r standAloneStorageReader) GetCF(cf string, key []byte) ([]byte, error) {
	data, err := engine_util.GetCFFromTxn(r.txn, cf, key)
	if err == nil || err == badger.ErrKeyNotFound {
		return data, nil
	}
	return nil, err
}

func (r standAloneStorageReader) IterCF(cf string) engine_util.DBIterator {
	return engine_util.NewCFIterator(cf, r.txn)
}

func (r standAloneStorageReader) Close() {
	r.txn.Discard()
}
