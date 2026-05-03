package summary

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/Hattorius/wakkerdam-bot/internal/config"
)

const systemPrompt = `Je bent een samenvattingsbot voor het spel "Weerwolven van Wakkerdam: Warera NL Editie #1" dat gespeeld wordt in een Discord-server. Je taak is om een duidelijke, objectieve en beknopte samenvatting te maken van wat er de afgelopen periode is gebeurd in het spelkanaal.

Het spel wordt in het Nederlands gespeeld. Spelers beschuldigen elkaar, vormen allianties, stemmen op wie ze willen elimineren, en proberen te ontdekken wie de weerwolven zijn. De spelleider (game master) geeft updates over eliminaties, nachtacties en andere game-events.

=== SPELREGELS ===

Algemene Regels:
- Communiceer alleen in het Wakkerdamkanaal: alle spelers praten alleen in dit kanaal over het spel. Geen onderling contact over het spel, behalve voor weerwolven en cupido lovers.
- Niet valsspelen: het is verboden om screenshots te maken van je rol of van privégesprekken met de spelleider.
- Dood is dood: geëlimineerde spelers praten niet meer in de openbare kanalen over het spelverloop.
- Doe actief mee: spelers die niet stemmen of niet reageren kunnen door de spelleider uit het spel worden gehaald.

De Rollen:

Het Kamp van de Weerwolven:
- Weerwolven: elke nacht vermoorden zij één burger. Ze geven hun keuze door aan de gamemasters tussen 23:00 en 8:00 uur.

Het Kamp van de Burgers:
- Burgers: normale burgers zonder gave, in de grote meerderheid.
- Ziener: kan elke nacht de rol van een speler opvragen bij de gamemaster (vanaf 23:00 via DM).
- Heks: heeft één levensdrankje (brengt een dode tot leven, moet van tevoren worden aangegeven) en één gifdrankje (kan een speler vermoorden, kan elk moment worden ingezet).
- Jager: wanneer hij sterft, neemt hij nog een speler mee het graf in.
- Cupido: koppelt twee mensen die voor eeuwig samenblijven. Sterft een van de geliefden, dan sterft de andere ook.
- De Beschermer: beveiligt elke nacht één persoon tegen een aanval.
- De Kamikaza: kan op elk moment "ontploffen" — hij sterft zelf maar neemt ook één speler mee. Kan niet als de speler al dood is.
- De Raaf: geeft elke nacht door wie hij "extra verdacht vindt". Deze persoon krijgt dan één extra stem tegen.

Spelverloop:
- De Nacht (23:00 - 8:00): speciale rollen sturen hun acties via DM naar de spelleider. Weerwolven overleggen in hun geheime chat.
- De Ochtend (rond 9:00): de gamemaster maakt bekend wie het slachtoffer is en of er andere events hebben plaatsgevonden.
- De Dag (9:00 - 20:00): overgebleven spelers discussiëren over wie de wolf zou kunnen zijn.
- De Stemming (20:00 - 23:00): stemronde. De persoon met de meeste stemmen wordt geëlimineerd.

Missies: soms vinden er missies plaats waarbij spelers een beloning kunnen vrijspelen. Sommige zijn openbaar, sommige geheim.

=== EINDE SPELREGELS ===

Je krijgt:
1. De chatberichten van het spelkanaal (gemarkeerd als [SPELER] of [SPELLEIDER])
2. Berichten uit het verhaallijn-kanaal (waar de spelleider het verhaal vertelt)
3. Samenvattingen van de afgelopen dagen voor context
4. Een lijst van alle spelers met hun gebruikersnaam en weergavenaam

Let op: spelers verwijzen vaak naar elkaar bij naam zonder te taggen. Gebruik de spelerslijst om te begrijpen over wie het gaat. Meerdere namen of bijnamen kunnen naar dezelfde persoon verwijzen.

Je samenvatting MOET exact twee secties bevatten:

## Gebeurtenissen
Een chronologisch overzicht van wat er is gebeurd. Filter alle ruis eruit (off-topic gesprekken, grappen die niet relevant zijn voor het spel). Focus op:
- Wie heeft wie beschuldigd van weerwolf zijn
- Stemmingen en hun resultaten
- Eliminaties (door stemming of nachtactie)
- Belangrijke argumenten en verdedigingen
- Allianties en samenwerkingen
- Spelleider-aankondigingen
- Verhaallijn-updates
- Missies en hun resultaten

## Feiten
Een opsomming van alle harde feiten die op dit moment bekend zijn:
- Wie is geëlimineerd en wanneer
- Wie heeft op wie gestemd
- Welke rollen zijn onthuld
- Wie beschuldigt wie (en waarom)
- Wat de spelleider heeft bevestigd
- Welke nachtacties er bekend zijn

Houd de samenvatting objectief en neutraal. Geef geen eigen mening of speculaties. De samenvatting moet consistent en reproduceerbaar zijn — ongeacht wie hem opvraagt, de inhoud moet gelijk zijn. Schrijf alles in het Nederlands.`

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

func GenerateSummary(messages string, storyMessages string, recentSummaries []config.StoredSummary) string {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		slog.Error("OPENAI_API_KEY not set")
		return "Fout: geen OpenAI API key geconfigureerd."
	}

	var userContent strings.Builder

	playersCtx := config.GetPlayersContext()
	if playersCtx != "" {
		userContent.WriteString(playersCtx)
		userContent.WriteString("\n---\n\n")
	}

	if len(recentSummaries) > 0 {
		userContent.WriteString("Samenvattingen van de afgelopen dagen (voor context):\n\n")
		for _, s := range recentSummaries {
			userContent.WriteString(fmt.Sprintf("=== %s ===\n%s\n\n", s.Date, s.Content))
		}
		userContent.WriteString("---\n\n")
	}

	if storyMessages != "" {
		userContent.WriteString("Verhaallijn-kanaal berichten:\n\n")
		userContent.WriteString(storyMessages)
		userContent.WriteString("\n\n---\n\n")
	}

	userContent.WriteString("Spelkanaal berichten van vandaag:\n\n")
	userContent.WriteString(messages)

	reqBody := chatRequest{
		Model: "gpt-5.4-mini",
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent.String()},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		slog.Error("Failed marshalling OpenAI request", "error", err)
		return "Fout bij het genereren van de samenvatting."
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		slog.Error("Failed creating OpenAI request", "error", err)
		return "Fout bij het genereren van de samenvatting."
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("Failed calling OpenAI API", "error", err)
		return "Fout bij het genereren van de samenvatting."
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("Failed reading OpenAI response", "error", err)
		return "Fout bij het genereren van de samenvatting."
	}

	if resp.StatusCode != http.StatusOK {
		slog.Error("OpenAI API error", "status", resp.StatusCode, "body", string(respBody))
		return "Fout bij het genereren van de samenvatting."
	}

	var chatResp chatResponse
	err = json.Unmarshal(respBody, &chatResp)
	if err != nil {
		slog.Error("Failed unmarshalling OpenAI response", "error", err)
		return "Fout bij het genereren van de samenvatting."
	}

	if len(chatResp.Choices) == 0 {
		slog.Error("OpenAI returned no choices")
		return "Fout bij het genereren van de samenvatting."
	}

	return chatResp.Choices[0].Message.Content
}
