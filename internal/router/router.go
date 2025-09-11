package router

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	cfg "github.com/cuongceg/validate_yaml/internal/config"
	util "github.com/cuongceg/validate_yaml/internal/util"
	"github.com/redis/go-redis/v9"
)

// ====== Message model trung gian ======
type Message struct {
	Value []byte
	Meta  map[string]any // size, createdAtMs, source_name, ...
}

// ====== Bus adapters (trừu tượng hóa connectors) ======
type Source interface {
	Subscribe(ctx context.Context, sourceName string, h func(context.Context, *Message) error) error
}
type Sink interface {
	Publish(ctx context.Context, targetName string, msg *Message) error
}
type Bus interface {
	Source
	Sink
	Close() error
}

// ====== Codec ======
type PayloadCodec interface {
	Decode(b []byte) (obj map[string]any, domain any, err error)
	Encode(obj map[string]any, origDomain any) ([]byte, error)
	ContentType() string
}

// ====== Filter/Projection ======
type FilterFn func(ctx context.Context, msg *Message, obj map[string]any) (bool, error)
type ProjectFn func(ctx context.Context, msg *Message, obj map[string]any) (map[string]any, error)

type Engine struct {
	Buses          map[string]Bus
	CodecsBySource PayloadCodec
	Filters        map[string]FilterFn
	Projections    map[string]ProjectFn
	Logger         func(format string, args ...any)

	// tune
	LanesPerTarget int // số lane publish mỗi target (mặc định 8)
	LaneBuffer     int // độ sâu buffer mỗi lane (mặc định 20000)
}

func (e *Engine) logf(format string, args ...any) {
	util.App.Printf(format, args...)
}

func (e *Engine) StartRoutes(ctx context.Context, uc *cfg.UserConfig, rdb *redis.Client) (stop func(), err error) {
	if e.Buses == nil {
		return nil, errors.New("router.Engine: Buses is nil")
	}
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)

	// defaults
	lanes := 16
	laneBuf := 8192

	// Map group_receivers
	type toTarget struct{ Connector, Target string }
	groupTargets := make(map[string][]toTarget)
	for _, g := range uc.GroupReceivers {
		var targets []toTarget
		for _, t := range g.Targets {
			targets = append(targets, toTarget{Connector: t.Connector, Target: t.Target})
		}
		groupTargets[g.Name] = targets
	}

	// Duyệt routes theo YAML
	for _, r := range uc.Routes {
		fromConn := r.From.Connector
		fromSrc := r.From.Source

		// Danh sách đích
		var targets []toTarget
		if r.ToGroup != "" {
			ts, ok := groupTargets[r.ToGroup]
			if !ok {
				cancel()
				return nil, fmt.Errorf("route %q: to_group %q not found", r.Name, r.ToGroup)
			}
			targets = ts
		} else {
			targets = []toTarget{{Connector: r.To.Connector, Target: r.To.Target}}
		}

		inBus, ok := e.Buses[fromConn]
		if !ok {
			cancel()
			return nil, fmt.Errorf("route %q: connector %q not found", r.Name, fromConn)
		}

		var codec PayloadCodec
		if e.CodecsBySource != nil {
			codec = e.CodecsBySource
		}

		// Filters
		var filterFns []FilterFn
		for _, name := range r.Filters {
			if f, ok := e.Filters[name]; ok {
				filterFns = append(filterFns, f)
			} else {
				cancel()
				return nil, fmt.Errorf("route %q: filter %q not registered", r.Name, name)
			}
		}

		// Projection
		var project ProjectFn
		if r.Projection != "" {
			p, ok := e.Projections[r.Projection]
			if !ok {
				cancel()
				return nil, fmt.Errorf("route %q: projection %q not registered", r.Name, r.Projection)
			}
			project = p
		}

		// Mode
		mode := strings.ToLower(r.Mode.Type) // "persistent" | "drop" | ...
		var ttlMs int64 = 0
		var maxAttempts int = 0
		if mode == "drop" {
			ttlMs = r.Mode.TTLms
			maxAttempts = r.Mode.MaxAttempts
		}

		// === Worker pool per-target ===
		type job struct {
			msg  *Message
			done chan error // báo về để commit offset sau khi publish OK
		}
		// lanes[targetIndex][laneIndex] -> chan job
		lanesPerTarget := make([][]chan job, len(targets))
		for ti := range targets {
			lanesPerTarget[ti] = make([]chan job, lanes)
			for li := 0; li < lanes; li++ {
				lanesPerTarget[ti][li] = make(chan job, laneBuf)
			}
		}

		// Spin worker cho từng lane/target
		for ti, tgt := range targets {
			outBus := e.Buses[tgt.Connector]
			if outBus == nil {
				cancel()
				return nil, fmt.Errorf("route %q: target connector %q not found", r.Name, tgt.Connector)
			}
			for li := 0; li < lanes; li++ {
				wg.Add(1)
				go func(routeName string, tgt toTarget, laneIdx int, q <-chan job) {
					defer wg.Done()
					for j := range q {
						var err error
						//attempt := 0
						for {
							// Publish blocking; exgress sẽ xử lý confirm/return
							err = outBus.Publish(ctx, tgt.Target, j.msg)
							break
							// if err == nil {
							// 	break
							// }
							// if ttlMs > 0 {
							// 	created, _ := getInt64(j.msg.Meta, "createdAtMs")
							// 	age := time.Now().UnixMilli() - created
							// 	if created > 0 && age > ttlMs {
							// 		err = nil // coi như drop-success để không giữ offset mãi
							// 		break
							// 	}
							// }
							// attempt++
							// if maxAttempts > 0 && attempt >= maxAttempts {
							// 	// hết nỗ lực
							// 	break
							// }
							// time.Sleep(50 * time.Millisecond)
							// continue
						}
						j.done <- err
					}
				}(r.Name, tgt, li, lanesPerTarget[ti][li])
			}
		}

		wg.Add(1)
		go func(routeName string, fromSrc string, inBus Bus) {
			fmt.Printf("[route=%s] start: from %s/%s to %d targets, lanes=%d, mode=%s, ttlMs=%d, maxAttempts=%d\n",
				routeName, fromConn, fromSrc, len(targets), lanes, mode, ttlMs, maxAttempts)
			defer wg.Done()

			// Subscribe nguồn; handler sẽ:
			// 1) decode/filter/project
			// 2) gửi job tới đúng lane của từng target
			// 3) CHỜ all targets ok -> return nil -> ingress commit offset
			if err := inBus.Subscribe(ctx, fromSrc, func(ctx context.Context, in *Message) error {
				//Decode (nếu có)
				// msg_id, has := in.Meta["msg_id"].(string)
				// if has {
				// 	ok, hopErr := util.CheckHop(ctx, rdb, msg_id, fromConn, fromSrc, time.Minute*10)
				// 	if !ok {
				// 		e.logf("[route=%s] drop msg_id=%s due to loop hop with error %s", routeName, in.Meta["msg_id"].(string), hopErr)
				// 		return nil
				// 	}
				// } else {
				// 	e.logf("[route=%s] warning: message without msg_id in meta", routeName)
				// }
				var obj map[string]any
				var domain any
				var err error
				if codec != nil && len(in.Value) > 0 {
					obj, domain, err = codec.Decode(in.Value)
					if err != nil {
						e.logf("[route=%s] decode error: %v", routeName, err)
						return fmt.Errorf("decode failed: %w", err)
					}
				}
				// meta
				if in.Meta == nil {
					in.Meta = map[string]any{}
				}

				// filter
				for _, f := range filterFns {
					ok, ferr := f(ctx, in, obj)
					if ferr != nil {
						e.logf("[route=%s] filter error: %v", routeName, ferr)
						return ferr
					}
					if !ok {
						return fmt.Errorf("filtered")
					}
				}

				// projection
				if project != nil && codec != nil {
					newObj, perr := project(ctx, in, obj)
					if perr != nil {
						e.logf("[route=%s] projection error: %v", routeName, perr)
						return perr
					}
					enc, eerr := codec.Encode(newObj, domain)
					if eerr != nil {
						e.logf("[route=%s] encode error: %v", routeName, eerr)
						return eerr
					}
					in.Value = enc
				}

				// Publish tới tất cả targets qua lanes và CHỜ kết quả,
				// để đảm bảo commit offset chỉ sau khi downstream OK.
				var wgPub sync.WaitGroup
				errs := make(chan error, len(targets))
				for ti, tgt := range targets {
					wgPub.Add(1)
					go func(ti int, tgt toTarget) {
						defer wgPub.Done()
						j := job{msg: in, done: make(chan error, 1)}
						li := pickLaneIndex(lanes)
						lanesPerTarget[ti][li] <- j
						err := <-j.done
						if err != nil {
							errs <- fmt.Errorf("%s/%s: %w", tgt.Connector, tgt.Target, err)
						} else {
							errs <- nil
						}
					}(ti, tgt)
				}
				wgPub.Wait()
				close(errs)

				// tổng hợp
				for e2 := range errs {
					if e2 != nil {
						return e2 // khiến ingress không commit; sẽ retry theo Kafka group
					}
				}

				return nil
			}); err != nil {
				e.logf("[route=%s] subscribe error on %s/%s: %v", routeName, fromConn, fromSrc, err)
				cancel()
				return
			}
			<-ctx.Done()
			// đóng các lane
			for ti := range lanesPerTarget {
				for li := 0; li < lanes; li++ {
					close(lanesPerTarget[ti][li])
				}
			}
		}(r.Name, fromSrc, inBus)
	}

	stop = func() {
		cancel()
		wg.Wait()
	}
	return stop, nil
}

func getInt64(m map[string]any, k string) (int64, bool) {
	if m == nil {
		return 0, false
	}
	switch v := m[k].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	}
	return 0, false
}

func pickLaneIndex(lanes int) int {
	if lanes <= 1 {
		return 0
	}
	return int(time.Now().UnixNano() % int64(lanes))
}
