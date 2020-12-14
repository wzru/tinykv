# Project1 StandaloneKV

Project1主要是实现单机KV存储引擎接口，其中`engine_util`包提供了一些有用的方法封装

## 运行逻辑

测试通过`go test`进行，针对每个project都有各自的测试用例。

其中project1的测试都在`kv/server/server_test.go`中，每个测试用例的主要逻辑都是新建一个单机KV引擎，然后对数据库进行CRUD，最后检查Get的结果和操作返回错误。

以下的proposal顺序大致是按照逻辑递进的。

## Storage实现

### NewStandAloneStorage(*config.Config) *StandAloneStorage

这个函数需要用给定的`Config`来新建一个`StandAloneStorage`，这里的话就构造一个StandAloneStorage即可。由于我们最终要调用的是`badger` API，所以之后需要构造一个`badger.DB`对象，构造`badger.DB`又需要`badger.Options`类型，因此这里的实现是将`Config`中的一些参数拷贝到`badger.Option`中，返回即可。

```go
//NewStandAloneStorage will create a StandAloneStorage
func NewStandAloneStorage(conf *config.Config) *StandAloneStorage {
	opt := badger.DefaultOptions
	opt.Dir = conf.DBPath
	opt.ValueDir = conf.DBPath
	return &StandAloneStorage{opt: opt}
}
```

### StandAloneStorage结构

因此我们可以确定`StandAloneStorage`结构：

```go
// StandAloneStorage is an implementation of `Storage` for a single-node TinyKV instance. It does not
// communicate with other nodes and all data is stored locally.
type StandAloneStorage struct {
	opt badger.Options
	db  *badger.DB
}
```

### (s *StandAloneStorage) Start() error

我的理解是这个函数需要连接到数据库，所以根据之前构造好的`badger.Options`调用`badger.Open()`函数连接/打开数据库即可。

```go
//Start will start the db
func (s *StandAloneStorage) Start() error {
	var err error
	s.db, err = badger.Open(s.opt)
	return err
}
```

### (s *StandAloneStorage) Stop() error

这个函数需要关闭服务，所以调用`badger.DB`的`Close`方法

```go
//Stop will close the db
func (s *StandAloneStorage) Stop() error {
	return s.db.Close()
}
```

### (s *StandAloneStorage) Reader( *kvrpcpb.Context ) (storage.StorageReader, error)

这个函数要获取一个`StorageReader`对象（来进行之后的读）。`StorageReader`是一个接口类型，定义了若干种方法，我认为也就是我们要再实现这个接口类型，这里我设计了`standAloneStorageReader`结构。

### standAloneStorageReader结构

根据文档的提示，`Reader`需要借助`badger.Txn`来实现，`Txn`是Transaction事务的缩写。所以这个结构中应该有`badger.Txn`

```go
type standAloneStorageReader struct {
	txn *badger.Txn
}
```

所以上述的`Reader`方法的实现，就是对应构造`badger.Txn`。

```go
//Reader returns a StorageReader
func (s *StandAloneStorage) Reader(ctx *kvrpcpb.Context) (storage.StorageReader, error) {
	return &standAloneStorageReader{txn: s.db.NewTransaction(false)}, nil
}
```

这里的构造方法`NewTransaction(update bool)`中有一个参数`update`，表示是否需要更新数据库，由于我们是一个`Reader`，所以不需要更新数据库，填`false`即可

接下来我们要实现`storage.StorageReader`接口定义的所有方法：

```go
type StorageReader interface {
	// When the key doesn't exist, return nil for the value
	GetCF(cf string, key []byte) ([]byte, error)
	IterCF(cf string) engine_util.DBIterator
	Close()
}
```

这里主要的实现基本都是找`engine_util`包中封装好的一些方法，值得注意的是`GetCF`方法要在`KeyNotFound`的时候返回值为`nil`的`value`

```go
func (r *standAloneStorageReader) GetCF(cf string, key []byte) ([]byte, error) {
	data, err := engine_util.GetCFFromTxn(r.txn, cf, key)
	if err == nil {
		return data, nil
	}
	if err == badger.ErrKeyNotFound {
		return nil, nil
	}
	return nil, err
}

func (r *standAloneStorageReader) IterCF(cf string) engine_util.DBIterator {
	return engine_util.NewCFIterator(cf, r.txn)
}

func (r *standAloneStorageReader) Close() {
	r.txn.Discard()
}
```

### (s *StandAloneStorage) Write( *kvrpcpb.Context, batch []storage.Modify) error

`Write`函数主要是向数据库写入更改，首先看一下这个`[]storage.Modify`类型，发现只是包含了一个接口类型，用来表示`Put`和`Delete`两种类型的修改。那么我们这里也根据两种类型调用对应的更改方法。

```go
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
```

最后才进行一次`WriteToDB`，这样的话可以提高效率

## Server实现

server部分主要是一个微服务的API接口部分，我们需要读取requst、调用上述实现好的接口并返回对应的response

### RawGet

从request中的上下文构造`Reader`，然后根据request中的`Cf`和`Key`读取对应的数据返回即可。注意当数据没找到时，要将response的`NotFound`属性设为`true`

```go
//RawGet process raw get API
func (server *Server) RawGet(_ context.Context, req *kvrpcpb.RawGetRequest) (*kvrpcpb.RawGetResponse, error) {
	reader, err := server.storage.Reader(req.Context)
	if err != nil {
		return nil, err
	}
	data, _ := reader.GetCF(req.Cf, req.Key)
	if data == nil {
		return &kvrpcpb.RawGetResponse{
			NotFound: true,
		}, nil
	}
	return &kvrpcpb.RawGetResponse{
		Value:    data,
		NotFound: false,
	}, nil
}
```

### RawPut

对应`Write`方法，填入对应参数来构造即可

```go
//RawPut process raw put API
func (server *Server) RawPut(_ context.Context, req *kvrpcpb.RawPutRequest) (*kvrpcpb.RawPutResponse, error) {
	err := server.storage.Write(req.Context, []storage.Modify{
		{Data: storage.Put{
			Key:   req.Key,
			Value: req.Value,
			Cf:    req.Cf,
		}}})
	if err != nil {
		return &kvrpcpb.RawPutResponse{
			Error: err.Error(),
		}, err
	}
	return &kvrpcpb.RawPutResponse{}, nil
}
```

### RawDelete

跟`RawPut`几乎一样

```go
//RawDelete process raw delete API
func (server *Server) RawDelete(_ context.Context, req *kvrpcpb.RawDeleteRequest) (*kvrpcpb.RawDeleteResponse, error) {
	err := server.storage.Write(req.Context, []storage.Modify{
		{Data: storage.Delete{
			Cf:  req.Cf,
			Key: req.Key,
		}}})
	if err != nil {
		return &kvrpcpb.RawDeleteResponse{
			Error: err.Error(),
		}, err
	}
	return &kvrpcpb.RawDeleteResponse{}, nil
}
```

### RawScan

`RawScan`是对一段区间的扫描，这里先调用`Reader.IterCF()`获得一个迭代器，然后不断推进迭代器即可，同时注意检查迭代器的有效性`Valid()`。

```go
//RawScan process raw scan API
func (server *Server) RawScan(_ context.Context, req *kvrpcpb.RawScanRequest) (*kvrpcpb.RawScanResponse, error) {
	reader, err := server.storage.Reader(req.Context)
	if err != nil {
		return nil, err
	}
	res := &kvrpcpb.RawScanResponse{}
	iter := reader.IterCF(req.Cf)
	iter.Seek(req.StartKey)
	for i := uint32(0); i < req.Limit; i++ {
		if iter.Valid() {
			key := iter.Item().Key()
			val, err := iter.Item().Value()
			if err != nil {
				return res, err
			}
			res.Kvs = append(res.Kvs, &kvrpcpb.KvPair{
				Key:   key,
				Value: val,
			})
			iter.Next()
		} else {
			break
		}
	}
	return res, nil
}
```