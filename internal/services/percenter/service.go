package percenter

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
	"time"

	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func GotDynamicGeoPercentPerSsp(ctx context.Context, conn *sql.DB, client bidEngineGrpc.BidEngineServiceClient) error {
	rows, err := conn.Query(QUERY)
	if err != nil {
		return fmt.Errorf("Cannot conn: %w", err)
	}
	defer rows.Close()

	var data []StatRecord
	for rows.Next() {
		var record StatRecord
		if err := rows.Scan(&record.Domain, &record.Geo, &record.Ts5, &record.TotalPrice, &record.TotalDSPPrice, &record.Frofit); err != nil {
			return fmt.Errorf("Cannot scan %w:", err)
		}
		data = append(data, record)
	}

	// Группировка временных меток
	tsMap := make(map[time.Time]int)
	var uniqueTS []time.Time

	for _, record := range data {
		if _, exists := tsMap[record.Ts5]; !exists {
			tsMap[record.Ts5] = 0
			uniqueTS = append(uniqueTS, record.Ts5)
		}
	}

	sort.Slice(uniqueTS, func(i, j int) bool {
		return uniqueTS[i].Before(uniqueTS[j])
	})

	c := 0
	for _, ts := range uniqueTS {
		if c < 5 {
			tsMap[ts] = 0
			c++
		} else {
			tsMap[ts] = 1
		}
	}

	// Добавление группы к данным
	type DataWithGroup struct {
		StatRecord
		Group int
	}

	var dataWithGroup []DataWithGroup
	for _, record := range data {
		dataWithGroup = append(dataWithGroup, DataWithGroup{
			StatRecord: record,
			Group:      tsMap[record.Ts5],
		})
	}

	// Группировка по domain, geo, group
	groupedMap := make(map[string]map[string]map[int]*GroupedData)

	for _, record := range dataWithGroup {
		if _, exists := groupedMap[record.Domain]; !exists {
			groupedMap[record.Domain] = make(map[string]map[int]*GroupedData)
		}
		if _, exists := groupedMap[record.Domain][record.Geo]; !exists {
			groupedMap[record.Domain][record.Geo] = make(map[int]*GroupedData)
		}
		if _, exists := groupedMap[record.Domain][record.Geo][record.Group]; !exists {
			groupedMap[record.Domain][record.Geo][record.Group] = &GroupedData{
				Domain: record.Domain,
				Geo:    record.Geo,
				Group:  record.Group,
			}
		}

		grouped := groupedMap[record.Domain][record.Geo][record.Group]
		grouped.TotalPrice += record.TotalPrice
		grouped.TotalDSPPrice += record.TotalDSPPrice
		grouped.Frofit += record.Frofit
		groupedMap[record.Domain][record.Geo][record.Group] = grouped
	}

	// Преобразование в срез для агрегации
	var groupedData []GroupedData
	for _, geoMap := range groupedMap {
		for _, groupMap := range geoMap {
			for _, grouped := range groupMap {
				groupedData = append(groupedData, *grouped)
			}
		}
	}

	// Агрегация по domain и geo
	aggMap := make(map[string]map[string][]float32)

	for _, grouped := range groupedData {
		if _, exists := aggMap[grouped.Domain]; !exists {
			aggMap[grouped.Domain] = make(map[string][]float32)
		}
		aggMap[grouped.Domain][grouped.Geo] = append(aggMap[grouped.Domain][grouped.Geo], grouped.Frofit)
	}

	// Создание итоговой структуры
	changes := make(map[string]*bidEngineGrpc.MapStringToFloat)

	for domain, geoMap := range aggMap {
		if _, exists := changes[domain]; !exists {
			changes[domain].Values = make(map[string]float32)
		}

		for geo, frofits := range geoMap {
			if len(frofits) >= 2 {
				if frofits[0] < frofits[1] {
					changes[domain].Values[geo] = 0.01
				} else {
					changes[domain].Values[geo] = -0.01
				}
			} else {
				changes[domain].Values[geo] = 0.0
			}
		}
	}

	in := &bidEngineGrpc.SspGeoPercentsRequest_V2_5{
		Changes: changes,
	}

	if _, err := client.ChangeSspGeoPercentsMap(ctx, in); err != nil {
		log.Println("Cannot ChangeSspGeoPercentsMap: %w", err)
	}

	// Вывод результата
	/*jsonData, _ := json.MarshalIndent(dItog, "", "  ")
	fmt.Printf("Result: %s\n", string(jsonData))*/
	return nil
}

func NewPercenter(
	addressOfBidEngine string,
) *TPercenter {
	return &TPercenter{
		addressOfBidEngine: addressOfBidEngine,
	}
}

func (s *TPercenter) GetGrpClient() (
	bidEngineGrpc.BidEngineServiceClient,
	func() error,
) {
	conn, err := grpc.NewClient(
		s.addressOfBidEngine,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}

	client := bidEngineGrpc.NewBidEngineServiceClient(conn)

	return client, conn.Close
}
