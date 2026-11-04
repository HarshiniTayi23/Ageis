// Package catalog will hold the snapshot catalog replicated across a
// 3-node cluster with Raft, so the recovery history survives the loss of any
// single node and has exactly one linear order.
//
// Status: planned for Milestone 3. Today the catalog is the hash-chained set
// of manifests written by package repo.
package catalog
