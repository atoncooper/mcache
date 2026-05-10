package cluster

import (
	"errors"
	"time"
)

// ---------------------------------------------------------------------------
// Hash operations
// ---------------------------------------------------------------------------

func (cm *ClusterManager) HSet(key, field, value string) (int, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.HSet(key, field, value)
}

func (cm *ClusterManager) HSetNX(key, field, value string) (bool, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return false, err
	}
	return n.Client.HSetNX(key, field, value)
}

func (cm *ClusterManager) HGet(key, field string) (string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return "", err
	}
	return n.Client.HGet(key, field)
}

func (cm *ClusterManager) HDel(key string, fields ...string) (int, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.HDel(key, fields...)
}

func (cm *ClusterManager) HExists(key, field string) (bool, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return false, err
	}
	return n.Client.HExists(key, field)
}

func (cm *ClusterManager) HGetAll(key string) (map[string]string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return nil, err
	}
	return n.Client.HGetAll(key)
}

func (cm *ClusterManager) HKeys(key string) ([]string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return nil, err
	}
	return n.Client.HKeys(key)
}

func (cm *ClusterManager) HVals(key string) ([]string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return nil, err
	}
	return n.Client.HVals(key)
}

func (cm *ClusterManager) HLen(key string) (int, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.HLen(key)
}

func (cm *ClusterManager) HStrLen(key, field string) (int, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.HStrLen(key, field)
}

func (cm *ClusterManager) HIncrBy(key, field string, delta int64) (int64, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.HIncrBy(key, field, delta)
}

func (cm *ClusterManager) HIncrByFloat(key, field string, delta float64) (float64, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.HIncrByFloat(key, field, delta)
}

func (cm *ClusterManager) HMGet(key string, fields ...string) ([]any, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return nil, err
	}
	return n.Client.HMGet(key, fields...)
}

func (cm *ClusterManager) HMSet(key string, fvPairs ...string) error {
	n, err := cm.mode.node(key)
	if err != nil {
		return err
	}
	return n.Client.HMSet(key, fvPairs...)
}

// ---------------------------------------------------------------------------
// List operations
// ---------------------------------------------------------------------------

func (cm *ClusterManager) LPush(key string, elems ...string) (int, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.LPush(key, elems...)
}

func (cm *ClusterManager) RPush(key string, elems ...string) (int, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.RPush(key, elems...)
}

func (cm *ClusterManager) LPop(key string) (string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return "", err
	}
	return n.Client.LPop(key)
}

func (cm *ClusterManager) RPop(key string) (string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return "", err
	}
	return n.Client.RPop(key)
}

func (cm *ClusterManager) LLen(key string) (int, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.LLen(key)
}

func (cm *ClusterManager) LRange(key string, start, stop int) ([]string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return nil, err
	}
	return n.Client.LRange(key, start, stop)
}

func (cm *ClusterManager) LIndex(key string, index int) (string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return "", err
	}
	return n.Client.LIndex(key, index)
}

func (cm *ClusterManager) LSet(key string, index int, value string) error {
	n, err := cm.mode.node(key)
	if err != nil {
		return err
	}
	return n.Client.LSet(key, index, value)
}

func (cm *ClusterManager) LRem(key string, count int, value string) (int, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.LRem(key, count, value)
}

func (cm *ClusterManager) LTrim(key string, start, stop int) error {
	n, err := cm.mode.node(key)
	if err != nil {
		return err
	}
	return n.Client.LTrim(key, start, stop)
}

func (cm *ClusterManager) LInsert(key string, before bool, pivot, value string) (int, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.LInsert(key, before, pivot, value)
}

func (cm *ClusterManager) BLPop(key string, timeout time.Duration) (string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return "", err
	}
	return n.Client.BLPop(key, timeout)
}

func (cm *ClusterManager) BRPop(key string, timeout time.Duration) (string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return "", err
	}
	return n.Client.BRPop(key, timeout)
}

func (cm *ClusterManager) LPos(key, value string, rank, count, maxLen int) ([]int, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return nil, err
	}
	return n.Client.LPos(key, value, rank, count, maxLen)
}

// ---------------------------------------------------------------------------
// Set operations
// ---------------------------------------------------------------------------

func (cm *ClusterManager) SAdd(key string, elems ...string) (int, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.SAdd(key, elems...)
}

func (cm *ClusterManager) SRem(key string, elems ...string) (int, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.SRem(key, elems...)
}

func (cm *ClusterManager) SIsMember(key, elem string) (bool, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return false, err
	}
	return n.Client.SIsMember(key, elem)
}

func (cm *ClusterManager) SMembers(key string) ([]string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return nil, err
	}
	return n.Client.SMembers(key)
}

func (cm *ClusterManager) SCard(key string) (int, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.SCard(key)
}

func (cm *ClusterManager) SPop(key string) (string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return "", err
	}
	return n.Client.SPop(key)
}

func (cm *ClusterManager) SRandMember(key string, count int) ([]string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return nil, err
	}
	return n.Client.SRandMember(key, count)
}

// SUnion routes by the first key. All keys must reside on the same node.
func (cm *ClusterManager) SUnion(keys ...string) ([]string, error) {
	if len(keys) == 0 {
		return nil, errors.New("at least one key required")
	}
	n, err := cm.mode.node(keys[0])
	if err != nil {
		return nil, err
	}
	return n.Client.SUnion(keys...)
}

// SInter routes by the first key.
func (cm *ClusterManager) SInter(keys ...string) ([]string, error) {
	if len(keys) == 0 {
		return nil, errors.New("at least one key required")
	}
	n, err := cm.mode.node(keys[0])
	if err != nil {
		return nil, err
	}
	return n.Client.SInter(keys...)
}

// SDiff routes by the first key.
func (cm *ClusterManager) SDiff(keys ...string) ([]string, error) {
	if len(keys) == 0 {
		return nil, errors.New("at least one key required")
	}
	n, err := cm.mode.node(keys[0])
	if err != nil {
		return nil, err
	}
	return n.Client.SDiff(keys...)
}

// ---------------------------------------------------------------------------
// Key management
// ---------------------------------------------------------------------------

func (cm *ClusterManager) Exists(key string) (bool, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return false, err
	}
	return n.Client.Exists(key)
}

func (cm *ClusterManager) Type(key string) (string, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return "", err
	}
	b, err := n.Client.Type(key)
	if err != nil {
		return "", err
	}
	switch b {
	case 0:
		return "none", nil
	case 1:
		return "string", nil
	case 2:
		return "set", nil
	case 3:
		return "hash", nil
	case 4:
		return "list", nil
	default:
		return "unknown", nil
	}
}

func (cm *ClusterManager) Expire(key string, seconds int64) (bool, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return false, err
	}
	return n.Client.Expire(key, seconds)
}

func (cm *ClusterManager) PExpire(key string, ms int64) (bool, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return false, err
	}
	return n.Client.PExpire(key, ms)
}

func (cm *ClusterManager) TTL(key string) (int64, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.TTL(key)
}

func (cm *ClusterManager) PTTL(key string) (int64, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return 0, err
	}
	return n.Client.PTTL(key)
}

func (cm *ClusterManager) Persist(key string) (bool, error) {
	n, err := cm.mode.node(key)
	if err != nil {
		return false, err
	}
	return n.Client.Persist(key)
}

// Keys returns all keys matching pattern across the cluster.
func (cm *ClusterManager) Keys(pattern string) ([]string, error) {
	return cm.mode.Keys(pattern)
}
