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

const systemPrompt = `Je bent een samenvattingsbot voor het spel "Weerwolven van Wakkerdam: Warera NL Editie #1" dat gespeeld wordt in een Discord-server. Je taak is om een gedetailleerde, objectieve samenvatting te maken van wat er de afgelopen periode is gebeurd in het spelkanaal. Volledigheid is belangrijker dan kortheid.

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
Een gedetailleerd chronologisch overzicht van wat er is gebeurd. Filter off-topic gesprekken en irrelevante grappen, maar laat NIETS weg dat met het spel te maken heeft. Wees UITGEBREID, niet beknopt. Beschrijf:
- Alle beschuldigingen: wie beschuldigt wie, en met welk argument
- Alle verdedigingen: wie verdedigt zich of anderen, en hoe
- Discussies en debatten: wie is het met wie eens/oneens en waarom
- Claims: als iemand claimt een bepaalde rol te zijn of informatie te hebben
- Stemmingen: wie stemt op wie, met welke motivatie
- Eliminaties: wat er precies gebeurde (door stemming of nachtactie)
- Spelleider-aankondigingen: letterlijk alles wat de spelleider zegt
- Verhaallijn-updates: wat er in het verhaallijn-kanaal is verteld
- Allianties en samenwerkingen: wie trekt met wie op
- Verdachte patronen: wie is stil, wie gedraagt zich anders dan normaal
- Missies en hun resultaten
- Vermeld tijdstippen waar relevant (ochtend, middag, avond, nacht)

## Feiten
Een complete opsomming van alle harde feiten die vandaag bekend zijn geworden:
- Wie is geëlimineerd en wanneer (en hoe: stemming, weerwolven, heks, jager, etc.)
- Alle uitgebrachte stemmen: wie stemde op wie
- Welke rollen zijn onthuld of geclaimd
- Alle beschuldigingen met hun onderbouwing
- Wat de spelleider heeft bevestigd of verduidelijkt
- Welke nachtacties er bekend zijn
- Welke spelmechanismen zijn besproken of verduidelijkt
- Status van speciale items (bijv. hoeveel drankjes de heks nog heeft)

REGELS:
- Herhaal GEEN informatie die al in eerdere samenvattingen staat. De eerdere samenvattingen zijn alleen voor jouw context. Focus op wat er NIEUW is.
- Zeg niets dubbel tussen de twee secties. Gebeurtenissen = het verhaal, Feiten = harde datapunten.
- Mis NIETS dat relevant is voor het spel. Het is beter om te veel te noemen dan te weinig.
- Als er veel is gebeurd, schrijf dan een lange samenvatting. Kortheid is NIET het doel.
- Gebruik de namen/weergavenamen van spelers consistent.

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
