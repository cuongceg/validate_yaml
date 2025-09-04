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
)

// ====== Message model trung gian ======
type Message struct {
	Key     []byte
	Value   []byte
	Headers map[string]string
	Meta    map[string]any // size, createdAtMs, source_name, ...
}

// ====== Bus adapters (trừu tượng hóa connectors) ======
type Source interface {
	// Subscribe vào một nguồn (source_name) đã khai báo trong connector.ingress.
	// Handler phải non-blocking (router sẽ tự chạy goroutine).
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

// ====== Codec cho payload (protobuf, v.v) ======
type PayloadCodec interface {
	// Decode: []byte -> (object map để filter/projection), và có thể là object domain
	Decode(b []byte) (obj map[string]any, domain any, err error)
	// Encode: obj map (sau projection) -> []byte protobuf
	Encode(obj map[string]any, origDomain any) ([]byte, error)
	ContentType() string // ví dụ "application/x-protobuf"
}

// ====== Filter/Projection tối thiểu ======
type FilterFn func(ctx context.Context, msg *Message, obj map[string]any) (bool, error)
type ProjectFn func(ctx context.Context, msg *Message, obj map[string]any) (map[string]any, error)

type Engine struct {
	Buses          map[string]Bus       // key = connector name
	CodecsBySource PayloadCodec         // key = from.source (source_name)
	Filters        map[string]FilterFn  // key = filter name
	Projections    map[string]ProjectFn // key = projection name
	Logger         func(format string, args ...any)
}

func (e *Engine) logf(format string, args ...any) {
	util.App.Printf(format, args...)
}

// ====== Khởi chạy tất cả routes theo YAML ======
func (e *Engine) StartRoutes(ctx context.Context, uc *cfg.UserConfig) (stop func(), err error) {
	if e.Buses == nil {
		return nil, errors.New("router.Engine: Buses is nil")
	}
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)

	// Map group_receivers
	groupTargets := make(map[string][]toTarget)
	for _, g := range uc.GroupReceivers {
		var targets []toTarget
		for _, t := range g.Targets {
			targets = append(targets, toTarget{
				Connector: t.Connector,
				Target:    t.Target,
			})
		}
		groupTargets[g.Name] = targets
	}

	// Duyệt routes
	for _, r := range uc.Routes {
		fromConn := r.From.Connector
		fromSrc := r.From.Source

		// Xác định danh sách đích (to hoặc to_group)
		var targets []toTarget
		if r.ToGroup != "" {
			ts, ok := groupTargets[r.ToGroup]
			if !ok {
				cancel()
				return nil, fmt.Errorf("route %q: to_group %q not found", r.Name, r.ToGroup)
			}
			targets = ts
		} else {
			targets = []toTarget{{
				Connector: r.To.Connector,
				Target:    r.To.Target,
			}}
		}

		bus, ok := e.Buses[fromConn]
		if !ok {
			cancel()
			return nil, fmt.Errorf("route %q: connector %q not found", r.Name, fromConn)
		}

		var codec PayloadCodec
		if e.CodecsBySource != nil {
			codec = e.CodecsBySource
		}

		// Chuẩn bị filter chain
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

		wg.Add(1)
		go func(routeName string, fromSrc string, targets []toTarget, bus Bus, codec PayloadCodec,
			filters []FilterFn, project ProjectFn, mode string, ttlMs int64, maxAttempts int) {
			fmt.Printf("[route=%s] start: from %s/%s to %d targets, mode=%s, ttlMs=%d, maxAttempts=%d\n",
				routeName, fromConn, fromSrc, len(targets), mode, ttlMs, maxAttempts)
			defer wg.Done()

			if err := bus.Subscribe(ctx, fromSrc, func(ctx context.Context, in *Message) error {
				start := time.Now()
				// Giải mã payload (nếu có codec)
				var obj map[string]any
				var domain any
				var err error

				if codec != nil && len(in.Value) > 0 {
					obj, domain, err = codec.Decode(in.Value)
					if err != nil {
						e.logf("[route=%s] decode error: %v", routeName, err)
						return nil
					}
				}

				// Thêm meta hỗ trợ filter
				if in.Meta == nil {
					in.Meta = map[string]any{}
				}
				in.Meta["source_name"] = fromSrc
				in.Meta["size"] = len(in.Value)
				in.Meta["content_type"] = codec.ContentType()

				// Filters
				for _, f := range filters {
					ok, ferr := f(ctx, in, obj)
					if ferr != nil {
						e.logf("[route=%s] filter error: %v", routeName, ferr)
						return nil
					}
					if !ok {
						e.logf("[route=%s] filtered out", routeName)
						return nil
					}
				}
				// Projection
				if project != nil && codec != nil {
					newObj, perr := project(ctx, in, obj)
					if perr != nil {
						e.logf("[route=%s] projection error: %v", routeName, perr)
						return nil
					}
					enc, eerr := codec.Encode(newObj, domain)
					if eerr != nil {
						e.logf("[route=%s] encode error: %v", routeName, eerr)
						return nil
					}
					in.Value = enc
				}
				e.logf("ready to publish, obj=%v", obj)

				// Publish tới các targets
				for _, t := range targets {
					outBus, ok := e.Buses[t.Connector]
					if !ok {
						e.logf("[route=%s] target connector %q not found", routeName, t.Connector)
						continue
					}
					// retry theo maxAttempts nếu mode persistent
					for {
						if err := outBus.Publish(ctx, t.Target, in); err != nil {
							if mode == "persistent" {
								time.Sleep(100 * time.Millisecond)
								continue
							} else if mode == "drop" {
								e.logf("[route=%s] publish error to %s/%s: %v", routeName, t.Connector, t.Target, err)
								if ttlMs > 0 {
									created, _ := getInt64(in.Meta, "createdAtMs")
									if created > 0 && (time.Now().UnixMilli()-created) > ttlMs {
										e.logf("[route=%s] drop by TTL (age=%dms, ttl=%dms)", routeName,
											time.Now().UnixMilli()-created, ttlMs)
										return nil
									}
									continue
								} else {
									break
								}
							}
						}
						e.logf("[route=%s] published succesfully to %s/%s", routeName, t.Connector, t.Target)
						break
					}
				}

				fmt.Printf("[route=%s] ok in %s\n", routeName, time.Since(start))
				return nil
			}); err != nil {
				e.logf("[route=%s] subscribe error on %s/%s: %v", routeName, fromConn, fromSrc, err)
				cancel() // có thể hủy toàn bộ nếu muốn
				return
			}

			<-ctx.Done()
		}(r.Name, fromSrc, targets, bus, codec, filterFns, project, mode, ttlMs, maxAttempts)
	}

	stop = func() {
		cancel()
		wg.Wait()
	}
	return stop, nil
}

type toTarget struct {
	Connector string
	Target    string
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
