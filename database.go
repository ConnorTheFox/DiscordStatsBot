package main

import (
	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
)

type stat struct {
	_id    bson.ObjectId
	ID     string
	Game   string
	Hours  float64
	Ignore bool
}

type icon struct {
	_id      bson.ObjectId
	Game     string
	Location string
	Color    string
}

type blacklist struct {
}

type datastore struct {
	session *mgo.Session
}

func (datastore *datastore) findOne(collectionName string, query bson.M, result interface{}) {
	db := datastore.session.Copy()
	defer db.Close()

	db.DB("").C(collectionName).Find(query).One(result)
}

func (datastore *datastore) findAll(collectionName string, query bson.M, results interface{}) {
	db := datastore.session.Copy()
	defer db.Close()

	db.DB("").C(collectionName).Find(query).All(results)
}

func (datastore *datastore) findAllSort(collectionName, sort string, query bson.M, results interface{}) {
	db := datastore.session.Copy()
	defer db.Close()

	db.DB("").C(collectionName).Find(query).Sort(sort).All(results)
}

func setUpDB() (*mgo.Session, *datastore) {
	session, err := mgo.Dial("localhost/statbot")
	if err != nil {
		panic(err)
	}

	genIndex := func(keys []string) mgo.Index {
		return mgo.Index{
			Key:        keys,
			Unique:     true,
			Background: false,
			Sparse:     true,
		}
	}

	statbotSession := session.Copy()
	defer statbotSession.Close()

	if err = statbotSession.DB("").C("gamestats").EnsureIndex(genIndex([]string{"name", "game", "id"})); err != nil {
		panic(err)
	}
	if err = statbotSession.DB("").C("gameicons").EnsureIndex(genIndex([]string{"location", "game", "color"})); err != nil {
		panic(err)
	}
	if err = statbotSession.DB("").C("iconblacklists").EnsureIndex(genIndex([]string{"game"})); err != nil {
		panic(err)
	}
	return session, &datastore{session: session}
}
