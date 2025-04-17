package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

const region = "America/Santiago"

var db *sql.DB

// --- Carga de configuración y conexión DB ---

func loadEnv() {
	if err := godotenv.Load(); err != nil {
		log.Println("No se pudo cargar .env; usando variables del sistema")
	}
}

func getDBURI() string {
	loadEnv()
	uri := os.Getenv("DB_URI")
	if uri == "" {
		log.Fatal("DB_URI no definida")
	}
	return uri
}

func connectDB() *sql.DB {
	if db != nil {
		return db
	}
	var err error
	db, err = sql.Open("postgres", getDBURI())
	if err != nil {
		log.Fatalf("Error conectando a la base: %v", err)
	}
	if err = db.Ping(); err != nil {
		log.Fatalf("Ping fallido: %v", err)
	}
	return db
}

// --- Tipos y estructuras ---

type Parameter struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type WhatsAppMessage struct {
	MessagingProduct string `json:"messaging_product"`
	RecipientType    string `json:"recipient_type"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Template         struct {
		Name     string `json:"name"`
		Language struct {
			Code string `json:"code"`
		} `json:"language"`
		Components []struct {
			Type       string      `json:"type"`
			Parameters []Parameter `json:"parameters"`
		} `json:"components"`
	} `json:"template"`
	Context struct {
		CampaignID int `json:"campaign_id"`
	} `json:"context"`
}

type ParticipantsStruct struct {
	ParticipantID  int             `json:"participant_id"`
	Phone          string          `json:"phone"`
	Name           string          `json:"name"`
	CampaignID     int             `json:"campaign_id"`
	Metadata       json.RawMessage `json:"metadata"`
	LastTry        *time.Time      `json:"last_try"`
	Tries          int             `json:"tries"`
	ConversationID *int            `json:"conversation_id"`
}

type Campaign struct {
	CampaignID     int    `json:"campaign_id"`
	CompanyID      string `json:"company_id"`
	Template       string `json:"template"`
	Retry          bool   `json:"retry"`
	NumberRetry    int    `json:"number_retry"`
	CompanyPhoneID string `json:"company_phone_id"`
}

type TemplateInfo struct {
	Language     string `json:"language"`
	ParamCount   int    `json:"param_count"`
	TemplateText string `json:"template_text"`
}

type TemplateResponseData struct {
	Data []struct {
		Language   string `json:"language"`
		Components []struct {
			Text    string `json:"text,omitempty"`
			Example *struct {
				BodyText [][]string `json:"body_text"`
			} `json:"example,omitempty"`
		} `json:"components"`
	} `json:"data"`
}

type SendTemplateParams struct {
	PhoneNumberID  string
	AccessToken    string
	RecipientPhone string
	TemplateName   string
	LanguageCode   string
	Parameters     []string
	ParamCount     int
}

type CompanyPhone struct {
	Token              string `json:"token"`
	PhoneID            string `json:"phone_id"`
	CompanyPhoneNumber string `json:"company_phone_number"`
	CompanyID          string `json:"company_id"`
}

// --- Funciones auxiliares ---

func messagePayload(to, templateName, templateLanguage string, parameters []Parameter, IDCampaign int) WhatsAppMessage {
	msg := WhatsAppMessage{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               to,
		Type:             "template",
		Context: struct {
			CampaignID int `json:"campaign_id"`
		}{CampaignID: IDCampaign},
	}
	msg.Template.Name = templateName
	msg.Template.Language.Code = templateLanguage
	msg.Template.Components = []struct {
		Type       string      `json:"type"`
		Parameters []Parameter `json:"parameters"`
	}{{Type: "body", Parameters: parameters}}
	return msg
}

func updateParticipantsForContact(db *sql.DB, id, tries int, lastTry string, conversationID int) error {
	_, err := db.Exec(
		"UPDATE participant_for_contact SET tries=$1, last_try=$2, conversation_id=$3 WHERE participant_id=$4",
		tries, lastTry, conversationID, id,
	)
	if err != nil {
		log.Printf("Error al actualizar participante %d: %v", id, err)
	}
	return err
}

func sendWhatsAppTemplate(params SendTemplateParams) (string, error) {
	var payload map[string]interface{}
	if params.ParamCount > 0 && len(params.Parameters) > 0 {
		comp := make([]map[string]interface{}, 0, 1)
		paramList := []map[string]interface{}{}
		for _, p := range params.Parameters {
			paramList = append(paramList, map[string]interface{}{"type": "text", "text": p})
		}
		comp = append(comp, map[string]interface{}{"type": "body", "parameters": paramList})
		payload = map[string]interface{}{
			"messaging_product": "whatsapp",
			"to":                params.RecipientPhone,
			"type":              "template",
			"template": map[string]interface{}{
				"name":       params.TemplateName,
				"language":   map[string]interface{}{"code": params.LanguageCode},
				"components": comp,
			},
		}
	} else {
		payload = map[string]interface{}{
			"messaging_product": "whatsapp",
			"to":                params.RecipientPhone,
			"type":              "template",
			"template": map[string]interface{}{
				"name":     params.TemplateName,
				"language": map[string]interface{}{"code": params.LanguageCode},
			},
		}
	}

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://graph.facebook.com/v21.0/%s/messages", params.PhoneNumberID)
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Add("Authorization", "Bearer "+params.AccessToken)
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("error %d: %s", resp.StatusCode, string(b))
	}

	var respData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return "", err
	}

	msgs, ok := respData["messages"].([]interface{})
	if !ok || len(msgs) == 0 {
		return "", nil
	}
	first, _ := msgs[0].(map[string]interface{})
	if id, ok := first["id"].(string); ok {
		return id, nil
	}
	return "", nil
}

func Select100Contacts(db *sql.DB) ([]ParticipantsStruct, error) {
	query := `SELECT participant_id, phone, name, campaign_id, metadata, last_try, tries, conversation_id
			  FROM participants_for_contact ORDER BY campaign_id ASC LIMIT 250`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ParticipantsStruct
	for rows.Next() {
		var p ParticipantsStruct
		var rawMeta []byte
		var lt sql.NullTime
		var cid sql.NullInt64

		if err := rows.Scan(&p.ParticipantID, &p.Phone, &p.Name, &p.CampaignID, &rawMeta, &lt, &p.Tries, &cid); err != nil {
			return nil, err
		}
		p.Metadata = rawMeta
		if lt.Valid {
			p.LastTry = &lt.Time
		}
		if cid.Valid {
			id := int(cid.Int64)
			p.ConversationID = &id
		}
		list = append(list, p)
	}
	return list, nil
}

func getCampaignInfo(db *sql.DB, campaignID int) (Campaign, error) {
	var c Campaign
	err := db.QueryRow(
		`SELECT campaign_id, company_id, company_phone_id, template, retry, number_retry
		 FROM campaigns WHERE campaign_id=$1`, campaignID,
	).Scan(&c.CampaignID, &c.CompanyID, &c.CompanyPhoneID, &c.Template, &c.Retry, &c.NumberRetry)
	if err == sql.ErrNoRows {
		return Campaign{CampaignID: campaignID}, nil
	}
	return c, err
}

func updateConversation(db *sql.DB, idConversation int, sender string) error {
	var tries int
	var totalMsg string
	err := db.QueryRow(
		"SELECT tries, total_message FROM conversations WHERE id_conversation=$1",
		idConversation,
	).Scan(&tries, &totalMsg)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	tm, _ := strconv.Atoi(totalMsg)
	now := time.Now()
	if tm == 1 {
		_, err = db.Exec(
			`UPDATE conversations SET last_message=$1, total_message=$2, first_user_response_date=$3
			 WHERE id_conversation=$4`,
			now, tm+1, now, idConversation,
		)
	} else {
		_, err = db.Exec(
			`UPDATE conversations SET last_message=$1, total_message=$2
			 WHERE id_conversation=$3`,
			now, tm+1, idConversation,
		)
	}
	return err
}

func getApikeyAndPhoneId(db *sql.DB, companyPhoneId string) (CompanyPhone, error) {
	var cp CompanyPhone
	err := db.QueryRow(
		`SELECT token, phone_id, phone_number, company_id
		 FROM companyphones WHERE wa_business_id=$1`, companyPhoneId,
	).Scan(&cp.Token, &cp.PhoneID, &cp.CompanyPhoneNumber, &cp.CompanyID)
	if err == sql.ErrNoRows {
		return CompanyPhone{}, nil
	}
	return cp, err
}

func createMessage(db *sql.DB, idConv int, content, phone, status, sender, waID, companyPhone string) (int, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	var msgID int
	err := db.QueryRow(
		`INSERT INTO messages (id_conversation, content, date_time, phone, status, sender, wa_id_conversation, company_phone)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING message_id`,
		idConv, content, now, phone, status, sender, waID, companyPhone,
	).Scan(&msgID)
	if err != nil {
		return 0, err
	}
	_ = updateConversation(db, idConv, sender)
	return msgID, nil
}

func convertMetadataToParameters(raw json.RawMessage, maxParams int) ([]Parameter, error) {
	var arr []map[string]interface{}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, err
	}
	params := []Parameter{}
	for _, entry := range arr {
		for _, v := range entry {
			params = append(params, Parameter{Type: "text", Text: fmt.Sprintf("%v", v)})
			if len(params) >= maxParams {
				return params, nil
			}
		}
	}
	return params, nil
}

func GetTemplateInfo(templateName, apiKey, companyPhoneID string) (TemplateInfo, error) {
	url := fmt.Sprintf(
		"https://graph.facebook.com/v17.0/%s/message_templates?name=%s",
		companyPhoneID, templateName,
	)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Authorization", "Bearer "+apiKey)
	req.Header.Add("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return TemplateInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return TemplateInfo{}, fmt.Errorf("status %d", resp.StatusCode)
	}
	var tr TemplateResponseData
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return TemplateInfo{}, err
	}
	if len(tr.Data) == 0 {
		return TemplateInfo{}, fmt.Errorf("no templates found")
	}
	data := tr.Data[0]
	ti := TemplateInfo{Language: data.Language}
	if len(data.Components) > 0 {
		ti.TemplateText = data.Components[0].Text
		if data.Components[0].Example != nil &&
			len(data.Components[0].Example.BodyText) > 0 {
			ti.ParamCount = len(data.Components[0].Example.BodyText[0])
		}
	}
	return ti, nil
}

func fromTemplateTextToRealText(templateText string, parameters []Parameter) string {
	out := templateText
	for i, p := range parameters {
		ph := fmt.Sprintf("{{%d}}", i+1)
		out = strings.ReplaceAll(out, ph, p.Text)
	}
	return out
}

// --- Lógica principal ---

func handler() error {
	start := time.Now()
	db := connectDB()
	log.Println("Conexión a la base de datos exitosa")
	loc, err := time.LoadLocation(region)
	if err != nil {
		loc = time.FixedZone(region, -3*3600)
	}
	log.Printf("Hora en Chile: %s", time.Now().In(loc).Format(time.RFC3339))

	participants, err := Select100Contacts(db)
	if err != nil {
		return err
	}
	if len(participants) == 0 {
		log.Println("No hay participantes para procesar")
		return nil
	}
	log.Println("Obteniendo información de campaña")

	camp, err := getCampaignInfo(db, participants[0].CampaignID)
	if err != nil {
		return err
	}
	company, err := getApikeyAndPhoneId(db, camp.CompanyPhoneID)
	if err != nil {
		return err
	}

	log.Println("ACA muere")

	tmpl, err := GetTemplateInfo(camp.Template, company.Token, camp.CompanyPhoneID)
	if err != nil {
		return err
	}

	for _, p := range participants {
		if p.CampaignID != camp.CampaignID {
			camp, _ = getCampaignInfo(db, p.CampaignID)
			company, _ = getApikeyAndPhoneId(db, camp.CompanyPhoneID)
			tmpl, _ = GetTemplateInfo(camp.Template, company.Token, camp.CompanyPhoneID)
		}

		var convID int
		if !camp.Retry {
			err := db.QueryRow(
				`INSERT INTO conversations 
				 (participant_id, campaign_id, start_date, status, tries, feeling, total_message, last_message, last_sender)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id_conversation`,
				p.ParticipantID, p.CampaignID, time.Now(), "activa", 1, "neutral", 1, time.Now(), company.CompanyID,
			).Scan(&convID)
			if err != nil {
				log.Printf("Crear conversación: %v", err)
				continue
			}
			params, err := convertMetadataToParameters(p.Metadata, tmpl.ParamCount)
			if err != nil {
				log.Printf("Convertir metadata: %v", err)
				continue
			}
			msgText := fromTemplateTextToRealText(tmpl.TemplateText, params)
			strParams := []string{}
			for _, v := range params {
				strParams = append(strParams, v.Text)
			}

			msgID, err := sendWhatsAppTemplate(SendTemplateParams{
				PhoneNumberID:  company.PhoneID,
				AccessToken:    company.Token,
				RecipientPhone: p.Phone,
				TemplateName:   camp.Template,
				LanguageCode:   tmpl.Language,
				Parameters:     strParams,
				ParamCount:     tmpl.ParamCount,
			})
			if err != nil {
				log.Printf("Enviar template: %v", err)
				continue
			}

			_, _ = createMessage(db, convID, msgText, p.Phone, "accepted", company.CompanyID, msgID, company.CompanyPhoneNumber)
			_, _ = db.Exec("DELETE FROM participants_for_contact WHERE participant_id=$1", p.ParticipantID)

		} else {
			// retry lógica
			if p.Tries == 0 {
				if err := db.QueryRow(
					`INSERT INTO conversations 
					 (participant_id, campaign_id, start_date, status, tries, feeling, total_message, last_message, last_sender)
					 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id_conversation`,
					p.ParticipantID, p.CampaignID, time.Now(), "activa", 1, "neutral", 1, time.Now(), company.CompanyID,
				).Scan(&convID); err != nil {
					log.Printf("Crear conversación retry: %v", err)
					continue
				}
			} else if p.ConversationID != nil {
				convID = *p.ConversationID
			} else {
				log.Printf("Falta conversation_id para retry: %d", p.ParticipantID)
				continue
			}

			if p.LastTry != nil && p.Tries > 0 {
				if time.Since(*p.LastTry).Hours() <= 24 {
					log.Printf("Menos de 24h desde último intento: %d", p.ParticipantID)
					continue
				}
			}

			params, err := convertMetadataToParameters(p.Metadata, tmpl.ParamCount)
			if err != nil {
				log.Printf("Convertir metadata retry: %v", err)
				continue
			}
			msgText := fromTemplateTextToRealText(tmpl.TemplateText, params)
			strParams := []string{}
			for _, v := range params {
				strParams = append(strParams, v.Text)
			}

			msgID, err := sendWhatsAppTemplate(SendTemplateParams{
				PhoneNumberID:  company.PhoneID,
				AccessToken:    company.Token,
				RecipientPhone: p.Phone,
				TemplateName:   camp.Template,
				LanguageCode:   tmpl.Language,
				Parameters:     strParams,
				ParamCount:     tmpl.ParamCount,
			})
			if err != nil {
				log.Printf("Enviar template retry: %v", err)
				continue
			}

			_, _ = createMessage(db, convID, msgText, p.Phone, "accepted", company.CompanyID, msgID, company.CompanyPhoneNumber)
			newTries := p.Tries + 1
			_ = updateParticipantsForContact(db, p.ParticipantID, newTries, time.Now().Format(time.RFC3339), convID)
			if newTries >= camp.NumberRetry {
				_, _ = db.Exec("DELETE FROM participants_for_contact WHERE participant_id=$1", p.ParticipantID)
			}
		}
	}

	log.Printf("Proceso completo en %s", time.Since(start))
	return nil
}

func main() {
	if err := handler(); err != nil {
		log.Fatalf("Error ejecutando script: %v", err)
	}
}
