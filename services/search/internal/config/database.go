package config

import (
	"net"
	"net/url"
	"strconv"
	"time"
)

func (d Database) URL(migrate bool) string {
	user, password := d.AppUser, d.AppPassword
	if migrate {
		user, password = d.OwnerUser, d.OwnerPassword
	}
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(d.Host, strconv.Itoa(d.Port)), Path: d.Name, User: url.UserPassword(user, password)}
	u.RawQuery = url.Values{"sslmode": {d.SSLMode}, "connect_timeout": {strconv.FormatInt(int64(d.ConnectTimeout/time.Second), 10)}}.Encode()
	return u.String()
}
