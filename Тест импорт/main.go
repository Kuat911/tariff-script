package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/go-resty/resty/v2"
	"github.com/xuri/excelize/v2"
)

const (
	apiBaseURL   = "http://192.168.1.38:8060/api/v1/calc/ktt"
	messageType  = "IMPORT"
	weight       = 20
	isCoverWagon = false
)

var excelFiles = []string{"Крытые Импорт.xlsx", "Платформы Импорт.xlsx", "Полувагоны Импорт.xlsx"}

type ApiResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Fees []struct {
			Name          string  `json:"name"`
			Cost          float64 `json:"cost"`
			TotalDistance int     `json:"totalDistance"`
		} `json:"fees"`
		Total        float64 `json:"total"`
		VAT          float64 `json:"vat"`
		TotalWithVAT float64 `json:"totalWithVAT"`
	} `json:"data"`
}

func main() {
	client := resty.New()
	logFile, err := os.Create("request_log.json")
	if err != nil {
		log.Fatalf("Ошибка создания файла: %v", err)
	}
	defer logFile.Close()

	mismatchFile, err := os.Create("mismatch_log.json")
	if err != nil {
		log.Fatalf("Ошибка создания файла: %v", err)
	}
	defer mismatchFile.Close()

	requestCount := 0

	for _, file := range excelFiles {
		data, err := readExcelData(file)
		if err != nil {
			log.Fatalf("Ошибка чтения файла %s: %v", file, err)
		}

		for distance, row := range data {
			for cargoCode, expectedValue := range row {
				requestCount++
				response, err := sendGetRequest(client, distance, cargoCode, file, logFile, requestCount)
				if err != nil {
					log.Printf("Ошибка отправки запроса: %v", err)
					continue
				}
				cost := response.Data.Fees[0].Cost
				if expectedValue != cost {
					logMismatch(mismatchFile, distance, cargoCode, file, expectedValue, cost, response, requestCount)
				}
			}
		}
	}
}

func readExcelData(filename string) (map[int]map[string]float64, error) {
	file, err := excelize.OpenFile(filename)
	if err != nil {
		return nil, err
	}

	data := make(map[int]map[string]float64)
	sheet := file.GetSheetName(file.GetActiveSheetIndex())
	rows, err := file.GetRows(sheet)
	if err != nil {
		return nil, err
	}

	distances := make([]int, len(rows)-1)
	for i, row := range rows[1:] {
		distance, err := strconv.Atoi(row[0])
		if err != nil {
			return nil, err
		}
		distances[i] = distance
		data[distance] = make(map[string]float64)
	}

	for j, header := range rows[0][1:] {
		for i, row := range rows[1:] {
			if row[j+1] == "" {
				continue
			}
			cargoCode := header
			value, err := strconv.ParseFloat(row[j+1], 64)
			if err != nil {
				return nil, err
			}
			data[distances[i]][cargoCode] = value
		}
	}

	return data, nil
}

func sendGetRequest(client *resty.Client, distance int, cargoCode, fileName string, logFile *os.File, requestCount int) (ApiResponse, error) {
	wagonKindId := 57
	switch fileName {
	case "Крытые Импорт.xlsx":
		wagonKindId = 57
	case "Платформы Импорт.xlsx":
		wagonKindId = 63
	case "Полувагоны Импорт.xlsx":
		wagonKindId = 75
	case "Полувагоны_и_Платформы_межобласть_измененный_семена_подсолнечника.xlsx":
		wagonKindId = 63
	}

	resp, err := client.R().
		SetQueryParam("totalDistance", strconv.Itoa(distance)).
		SetQueryParam("cargoCode", cargoCode).
		SetQueryParam("wagonKindId", strconv.Itoa(wagonKindId)).
		SetQueryParam("messageType", messageType).
		SetQueryParam("weight", strconv.Itoa(weight)).
		SetQueryParam("isCoverWagon", strconv.FormatBool(isCoverWagon)).
		Get(apiBaseURL)
	if err != nil {
		return ApiResponse{}, err
	}

	var result ApiResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return ApiResponse{}, err
	}

	requestInfo := map[string]interface{}{
		"request_number": requestCount,
		"request_url":    resp.Request.URL,
		"response_body":  result,
	}
	logData, err := json.Marshal(requestInfo)
	if err != nil {
		log.Printf("Ошибка преобразования данных в JSON: %v", err)
	}
	log.Printf("Запрос #%d: %s", requestCount, logData)
	if _, err := logFile.WriteString(fmt.Sprintf("%s\n", logData)); err != nil {
		log.Printf("Ошибка записи в файл: %v", err)
	}

	return result, nil
}

func logMismatch(mismatchFile *os.File, distance int, cargoCode, fileName string, expectedValue, responseValue float64, response ApiResponse, requestCount int) {
	mismatchData := map[string]interface{}{
		"request_number": requestCount,
		"total_distance": distance,
		"cargo_code":     cargoCode,
		"file_name":      fileName,
		"expected_value": expectedValue,
		"response_value": responseValue,
		"api_response":   response,
	}
	data, err := json.Marshal(mismatchData)
	if err != nil {
		log.Printf("Ошибка преобразования данных в JSON: %v", err)
		return
	}
	log.Printf("Несовпадение #%d: %s", requestCount, data)
	if _, err := mismatchFile.WriteString(fmt.Sprintf("%s\n", data)); err != nil {
		log.Printf("Ошибка записи в файл: %v", err)
	}
}
