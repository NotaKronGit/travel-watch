import { TripStatus } from './gen/travelwatch/cabinet/v1/trips_pb';

// Terminal statuses never change back; expiry, unlike cancellation, keeps results viewable.
export function isTerminal(status: TripStatus) {
  return status === TripStatus.CANCELLED || status === TripStatus.COMPLETED || status === TripStatus.EXPIRED;
}
