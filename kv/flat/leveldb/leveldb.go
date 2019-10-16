// Copyright 2017 The Cayley Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package leveldb

import (
	"context"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/hidal-go/hidalgo/base"
	"github.com/hidal-go/hidalgo/kv/flat"
)

const (
	Name = "leveldb"
)

func init() {
	flat.Register(flat.Registration{
		Registration: base.Registration{
			Name: Name, Title: "LevelDB",
			Local: true,
		},
		OpenPath: OpenPath,
	})
}

var _ flat.KV = (*DB)(nil)

func New(d *leveldb.DB) *DB {
	return &DB{db: d}
}

func Open(path string, opt *opt.Options) (*DB, error) {
	db, err := leveldb.OpenFile(path, opt)
	if err != nil {
		return nil, err
	}
	return New(db), nil
}

func OpenPath(path string) (flat.KV, error) {
	db, err := Open(path, nil)
	if err != nil {
		return nil, err
	}
	return db, nil
}

type DB struct {
	db *leveldb.DB
	wo *opt.WriteOptions
	ro *opt.ReadOptions
}

func (db *DB) SetWriteOptions(wo *opt.WriteOptions) {
	db.wo = wo
}

func (db *DB) SetReadOptions(ro *opt.ReadOptions) {
	db.ro = ro
}
func (db *DB) Close() error {
	return db.db.Close()
}
func (db *DB) Tx(rw bool) (flat.Tx, error) {
	tx := &Tx{db: db}
	var err error
	if rw {
		tx.tx, err = db.db.OpenTransaction()
	} else {
		tx.sn, err = db.db.GetSnapshot()
	}
	if err != nil {
		return nil, err
	}
	return tx, nil
}
func (db *DB) View(fn func(tx flat.Tx) error) error {
	tx, _ := db.Tx(false)
	return fn(tx)
}

func (db *DB) Update(fn func(tx flat.Tx) error) error {
	tx, _ := db.Tx(true)
	return fn(tx)
}

type Tx struct {
	db  *DB
	sn  *leveldb.Snapshot
	tx  *leveldb.Transaction
	err error
}

func (tx *Tx) Commit(ctx context.Context) error {
	if tx.err != nil {
		return tx.err
	}
	if tx.tx != nil {
		tx.err = tx.tx.Commit()
		return tx.err
	}
	tx.sn.Release()
	return tx.err
}
func (tx *Tx) Close() error {
	if tx.tx != nil {
		tx.tx.Discard()
	} else {
		tx.sn.Release()
	}
	return tx.err
}
func (tx *Tx) Get(ctx context.Context, key flat.Key) (flat.Value, error) {
	var (
		val []byte
		err error
	)
	if tx.tx != nil {
		val, err = tx.tx.Get(key, tx.db.ro)
	} else {
		val, err = tx.sn.Get(key, tx.db.ro)
	}
	if err == leveldb.ErrNotFound {
		return nil, flat.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return val, nil
}
func (tx *Tx) GetBatch(ctx context.Context, keys []flat.Key) ([]flat.Value, error) {
	return flat.GetBatch(ctx, tx, keys)
}
func (tx *Tx) Put(k flat.Key, v flat.Value) error {
	if tx.tx == nil {
		return flat.ErrReadOnly
	}
	return tx.tx.Put(k, v, tx.db.wo)
}
func (tx *Tx) Del(k flat.Key) error {
	if tx.tx == nil {
		return flat.ErrReadOnly
	}
	return tx.tx.Delete(k, tx.db.wo)
}
func (tx *Tx) Scan(opt *flat.ScanOptions) flat.Iterator {
	r, ro := util.BytesPrefix(flat.KeyEscape(opt.Prefix)), tx.db.ro
	var it iterator.Iterator
	if tx.tx != nil {
		it = tx.tx.NewIterator(r, ro)
	} else {
		it = tx.sn.NewIterator(r, ro)
	}
	return &Iterator{it: it, first: true}
}

type Iterator struct {
	it    iterator.Iterator
	first bool
}

func (it *Iterator) Next(ctx context.Context) bool {
	if it.first {
		it.first = false
		return it.it.First()
	}
	return it.it.Next()
}
func (it *Iterator) Key() flat.Key   { return it.it.Key() }
func (it *Iterator) Val() flat.Value { return it.it.Value() }
func (it *Iterator) Err() error {
	return it.it.Error()
}
func (it *Iterator) Close() error {
	it.it.Release()
	return it.Err()
}
