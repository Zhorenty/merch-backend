package loyalty

import (
	"fmt"
	"strings"

	"merch/backend/internal/store"
)

// ProgramDescription is the back-of-card text: earn and redeem rules plus store locations.
func ProgramDescription(s Settings, stores []store.StoreRow, support string) string {
	s = s.withDefaults()
	per100 := 100 * s.EarnPercent / 100
	var b strings.Builder
	fmt.Fprintf(&b, "1 балл = %d ₽ скидки.\n", s.RedeemRate)
	fmt.Fprintf(&b, "С покупки начисляется %d%%: каждые 100 ₽ дают %d %s. Дробная часть отбрасывается. Баллы, которыми оплатили часть чека, в начисление не входят.\n",
		s.EarnPercent, per100, pointsWord(per100))
	if s.EarnMinReceipt > 0 {
		fmt.Fprintf(&b, "Начисление начинается с чека %d ₽.\n", s.EarnMinReceipt)
	}
	fmt.Fprintf(&b, "Списать можно от %d %s и не больше %d%% суммы чека.\n", s.RedeemMin, pointsWord(s.RedeemMin), s.RedeemMaxShare)
	if s.ExpireDays != nil && *s.ExpireDays > 0 {
		fmt.Fprintf(&b, "Баллы сгорают через %d %s.\n", *s.ExpireDays, daysWord(*s.ExpireDays))
	} else {
		b.WriteString("Баллы не сгорают.\n")
	}
	b.WriteString("Карта не платёжное средство: на кассе покажите QR. При возврате чека начисленные баллы снимаются, списанные возвращаются.\n")
	b.WriteString(formatStores(stores))
	if support = strings.TrimSpace(support); support != "" {
		fmt.Fprintf(&b, "\nВопросы: %s.", support)
	}
	return b.String()
}

func formatStores(stores []store.StoreRow) string {
	var lines []string
	for _, row := range stores {
		name := strings.TrimSpace(row.Name)
		addr := strings.TrimSpace(row.Address)
		switch {
		case name != "" && addr != "":
			lines = append(lines, name+" — "+addr)
		case name != "":
			lines = append(lines, name)
		case addr != "":
			lines = append(lines, addr)
		}
	}
	if len(lines) == 0 {
		return "Точки: список появится, когда магазин добавит адреса."
	}
	return "Точки:\n" + strings.Join(lines, "\n")
}

func pointsWord(n int) string {
	return plural(n, "балл", "балла", "баллов")
}

func daysWord(n int) string {
	return plural(n, "день", "дня", "дней")
}

func plural(n int, one, few, many string) string {
	if n < 0 {
		n = -n
	}
	mod100 := n % 100
	mod10 := n % 10
	if mod100 >= 11 && mod100 <= 14 {
		return many
	}
	switch mod10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}
