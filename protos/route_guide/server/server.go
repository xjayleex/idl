package server

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/golang/protobuf/proto"
	pb "google.golang.org/grpc/examples/route_guide/routeguide"
	"google.golang.org/grpc"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net"
	"sync"
	"time"
)
var (
	tls = flag.Bool("tls", false, "If set true, uses TLS, else use TCP")
	certFile = flag.String("cert_file", "", "TLS cert file")
	keyFile = flag.String("key_file", "", "TLS key file")
	jsonDBFile = flag.String("json_db_file","", "")
	port = flag.Int("port", 10000, "Server port")
)
type routeGuideServer struct {
	savedFeatures []*pb.Feature
	mu	sync.Mutex
	routeNotes map[string][]*pb.RouteNote
}

//	type RouteGuideServer interface
//	GetFeature(context.Context, *Point) (*Feature, error)
//	ListFeatures(*Rectangle, RouteGuide_ListFeaturesServer) error
//	RecordRoute(RouteGuide_RecordRouteServer) error
//	RouteChat(RouteGuide_RouteChatServer) error
//

// Simple RPC
func (s *routeGuideServer) GetFeature(ctx context.Context, point *pb.Point) (*pb.Feature, error) {
	for _, feature := range s.savedFeatures {
		if proto.Equal(feature.Location, point) {
			return feature, nil
		}
	}
	return &pb.Feature{Location: point}, nil
}

// Server-side streaming RPC
func (s *routeGuideServer) ListFeatures(rect *pb.Rectangle, stream pb.RouteGuide_ListFeaturesServer) error {
	for _, feature := range s.savedFeatures {
		if inRange(feature.Location, rect) {
			if err := stream.Send(feature); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *routeGuideServer) RecordRoute(stream pb.RouteGuide_RecordRouteServer) error {
	var pointCount, featureCount, distance int32
	var lastPoint *pb.Point
	startTime := time.Now()
	for {
		point, err := stream.Recv()
		if err == io.EOF {
			endTime := time.Now()
			return stream.SendAndClose(&pb.RouteSummary{
				PointCount: pointCount,
				FeatureCount: featureCount,
				Distance: distance,
				ElapsedTime: int32(endTime.Sub(startTime).Seconds()),
			})
		}
		if err != nil {
			return err
		}
		pointCount += 1
		for _, feature := range s.savedFeatures {
			if proto.Equal(feature.Location, point) {
				featureCount += 1
			}
		}
		if lastPoint != nil {
			distance += calcDistance(lastPoint, point)
		}

		lastPoint = point
	}
}

func (s *routeGuideServer) RouteChat(stream pb.RouteGuide_RouteChatServer) error{
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		key := serialize(in.Location)
		s.mu.Lock()
		s.routeNotes[key] = append(s.routeNotes[key],in)
		rn := make([]*pb.RouteNote, len(s.routeNotes[key]))
		copy(rn, s.routeNotes[key])
		s.mu.Unlock()
		for _, note := range rn {
			if err := stream.Send(note); err != nil {
				return err
			}
		}
	}
}

func (s *routeGuideServer) loadFeatures(filePath string) {
	var data [] byte
	if filePath != "" {
		var err error
		data, err = ioutil.ReadFile(filePath)
		if err != nil {
			log.Fatalf("Failed to load default features : %v", err)
		}
		if err := json.Unmarshal(data, &s.savedFeatures); err != nil {
			log.Fatalf("Failed to load default features: %v", err)
		}
	}
}

func inRange(point *pb.Point, rect *pb.Rectangle) bool {
	left := math.Min(float64(rect.Lo.Longitude), float64(rect.Hi.Longitude))
	right := math.Max(float64(rect.Lo.Longitude), float64(rect.Hi.Longitude))
	top := math.Max(float64(rect.Lo.Latitude), float64(rect.Hi.Latitude))
	bottom := math.Min(float64(rect.Lo.Latitude), float64(rect.Hi.Latitude))

	if float64(point.Longitude) >= left &&
		float64(point.Longitude) <= right &&
		float64(point.Latitude) >= bottom &&
		float64(point.Latitude) <= top {
		return true
	}
	return false
}

// calcDistance calculates the distance between two points using the "haversine" formula.
// The formula is based on http://mathforum.org/library/drmath/view/51879.html.
func calcDistance(p1 *pb.Point, p2 *pb.Point) int32 {
	const CordFactor float64 = 1e7
	const R = float64(6371000) // earth radius in metres
	lat1 := toRadians(float64(p1.Latitude) / CordFactor)
	lat2 := toRadians(float64(p2.Latitude) / CordFactor)
	lng1 := toRadians(float64(p1.Longitude) / CordFactor)
	lng2 := toRadians(float64(p2.Longitude) / CordFactor)
	dlat := lat2 - lat1
	dlng := lng2 - lng1

	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1)*math.Cos(lat2)*
			math.Sin(dlng/2)*math.Sin(dlng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	distance := R * c
	return int32(distance)
}

func serialize(point *pb.Point) string {
	return fmt.Sprint("%d %d", point.Latitude, point.Longitude)
}

func toRadians(num float64) float64 {
	return num * math.Pi / float64(180)
}

func newServer() *routeGuideServer {
	s := &routeGuideServer {routeNotes: make(map[string][]*pb.RouteNote)}
	s.loadFeatures(*jsonDBFile)
	return s
}

func main() {
	flag.Parse()
	lis ,err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
		log.Fatalf("failed to listen : %v", err)
	}
	var opts []grpc.ServerOption
	if *tls {

	}
	grpcServer := grpc.NewServer(opts ...)
	pb.RegisterRouteGuideServer(grpcServer, newServer().Svc())

}