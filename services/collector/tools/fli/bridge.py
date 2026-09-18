"""One bounded request on stdin, one sanitized response on stdout. No booking calls."""
import json
import logging
import sys
from datetime import datetime, timezone

logging.disable(logging.CRITICAL)


def search(query):
    from fli.models import Airport, FlightSearchFilters, FlightSegment, PassengerInfo, MaxStops
    from fli.search import SearchFlights

    if query['from'] not in Airport.__members__ or query['to'] not in Airport.__members__:
        return {'version': 1, 'error': 'unsupported'}
    filters = FlightSearchFilters(
        passenger_info=PassengerInfo(adults=query['adults']),
        flight_segments=[FlightSegment(
            departure_airport=[[Airport[query['from']], 0]],
            arrival_airport=[[Airport[query['to']], 0]], travel_date=query['date'])],
        stops=MaxStops.NON_STOP if query['nonstop'] else MaxStops.ANY,
    )
    found = SearchFlights().search(filters, currency='USD', language='en', country='US') or []
    options = []
    for item in found[:query['limit']]:
        options.append({'duration_minutes': item.duration, 'legs': [
            {'from': leg.departure_airport.name, 'to': leg.arrival_airport.name,
             'number': leg.airline.name.removeprefix('_') + leg.flight_number,
             'departure_local': leg.departure_datetime.isoformat(timespec='seconds'),
             'arrival_local': leg.arrival_datetime.isoformat(timespec='seconds')}
            for leg in item.legs]})
    return {'version': 1, 'result': {
        'provider': 'google-flights-fli', 'observed_at': datetime.now(timezone.utc).isoformat(),
        'query': query, 'complete': False, 'limit_reached': len(found) > query['limit'],
        'options': options,
        'warnings': ['Unofficial Google Flights integration; coverage and parsing completeness are not guaranteed.',
                     'Local airport times have no verified UTC offset; connections are not validated.',
                     'Prices, baggage and booking availability are not normalized in this experiment.']}}


def main():
    try:
        data = sys.stdin.buffer.read(4097)
        if len(data) > 4096:
            raise ValueError('input limit')
        result = search(json.loads(data))
    except Exception as exc:
        from fli.search.exceptions import SearchHTTPError
        code = 'rate_limited' if isinstance(exc, SearchHTTPError) and exc.status_code == 429 else 'unavailable'
        if type(exc).__name__ == 'SearchParseError':
            code = 'invalid_response'
        if type(exc).__name__ in ('ValidationError', 'KeyError', 'ValueError'):
            code = 'unsupported'
        result = {'version': 1, 'error': code}
    print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    main()
