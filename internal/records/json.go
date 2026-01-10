package records

import (
	"fmt"
	"time"

	"github.com/qdm12/ddns-updater/internal/constants"
	"github.com/qdm12/ddns-updater/internal/models"
)

func (r *Record) JSON(now time.Time) models.Stat {
	const NotAvailable = "N/A"
	row := models.Stat{
		Domain:      r.Provider.Domain(),
		Owner:       r.Provider.Owner(),
		Provider:    r.Provider.String(),
		IPVersion:   r.Provider.IPVersion().String(),
		CurrentIP:   r.History.GetCurrentIP().String(),
		PreviousIPs: r.History.GetPreviousIPs(),
	}
	message := r.Message
	if r.Status == constants.UPTODATE {
		message = "no IP change for " + r.History.GetDurationSinceSuccess(now)
	}
	if message != "" {
		message = fmt.Sprintf("(%s)", message)
	}
	if r.Status == "" {
		row.Status = NotAvailable
	} else {
		row.Status = fmt.Sprintf("%s %s, %s",
			r.Status,
			message,
			time.Since(r.Time).Round(time.Second).String()+" ago")
	}
	return row
}
