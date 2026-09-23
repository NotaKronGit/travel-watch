// Formats whole minutes as hours and minutes: 661 → "11 ч 1 мин", 180 → "3 ч".
export function formatMinutes(total: number | bigint): string {
  const value = Math.max(0, Number(total));
  const hours = Math.floor(value / 60);
  const minutes = value % 60;
  if (hours === 0) return `${minutes} мин`;
  return minutes === 0 ? `${hours} ч` : `${hours} ч ${minutes} мин`;
}
