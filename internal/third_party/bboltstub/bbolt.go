package bbolt

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

type Options struct{ Timeout time.Duration }
type DB struct{ path string; mu sync.Mutex; data map[string]map[string][]byte }
type Tx struct{ db *DB; writable bool }
type Bucket struct{ tx *Tx; name string }
type Cursor struct{ items [][2][]byte; idx int }
var ErrBucketNotFound = errors.New("bucket not found")
func Open(path string, mode os.FileMode, opts *Options)(*DB,error){ db:=&DB{path:path,data:map[string]map[string][]byte{}}; b,_:=os.ReadFile(path); if len(b)>0{_ = json.Unmarshal(b,&db.data)}; return db,nil }
func (db *DB) Close() error { return db.persist() }
func (db *DB) persist() error { b,err:=json.Marshal(db.data); if err!=nil{return err}; return os.WriteFile(db.path,b,0600) }
func (db *DB) Update(fn func(*Tx) error) error { db.mu.Lock(); defer db.mu.Unlock(); tx:=&Tx{db:db,writable:true}; if err:=fn(tx); err!=nil{return err}; return db.persist() }
func (db *DB) View(fn func(*Tx) error) error { db.mu.Lock(); defer db.mu.Unlock(); return fn(&Tx{db:db}) }
func (tx *Tx) CreateBucketIfNotExists(name []byte)(*Bucket,error){ n:=string(name); if tx.db.data[n]==nil{tx.db.data[n]=map[string][]byte{}}; return &Bucket{tx:tx,name:n},nil }
func (tx *Tx) Bucket(name []byte)*Bucket{ n:=string(name); if tx.db.data[n]==nil{return nil}; return &Bucket{tx:tx,name:n} }
func (b *Bucket) Put(k,v []byte) error { cp:=append([]byte(nil),v...); b.tx.db.data[b.name][string(k)]=cp; return nil }
func (b *Bucket) Get(k []byte) []byte { v:=b.tx.db.data[b.name][string(k)]; return append([]byte(nil),v...) }
func (b *Bucket) Delete(k []byte) error { delete(b.tx.db.data[b.name],string(k)); return nil }
func (b *Bucket) Cursor()*Cursor{ c:=&Cursor{}; for k,v:=range b.tx.db.data[b.name]{ c.items=append(c.items,[2][]byte{[]byte(k),append([]byte(nil),v...)})}; return c }
func (c *Cursor) First()(k,v []byte){ c.idx=0; if len(c.items)==0{return nil,nil}; return c.items[0][0],c.items[0][1] }
func (c *Cursor) Next()(k,v []byte){ c.idx++; if c.idx>=len(c.items){return nil,nil}; return c.items[c.idx][0],c.items[c.idx][1] }
