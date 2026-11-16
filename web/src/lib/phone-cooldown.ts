// This is a local cooldown key, not a substitute for server phone validation.
// Mainland China aliases match NormalizePhone; other regions retain explicit
// international prefixes and formatting normalization for bridge deployments.
export function phoneCooldownKey(value: string, country = "CN") {
  const region = country.trim().toUpperCase() || "CN";
  let number = value.trim().replace(/[\s().-]/g, "");
  if (region === "CN") {
    if (number.startsWith("+86")) number = number.slice(3);
    else if (number.startsWith("0086")) number = number.slice(4);
    else if (number.startsWith("86") && number.length === 13)
      number = number.slice(2);
    if (/^1[3-9]\d{9}$/.test(number)) number = `+86${number}`;
  } else if (number.startsWith("00")) {
    number = `+${number.slice(2)}`;
  }
  return `${region}:${number}`;
}

export function phoneRetrySeconds(value: unknown, fallback = 60) {
  return typeof value === "number" &&
    Number.isSafeInteger(value) &&
    value > 0 &&
    value <= 86400
    ? value
    : fallback;
}
