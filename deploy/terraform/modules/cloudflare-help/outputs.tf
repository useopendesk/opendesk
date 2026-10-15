output "cname_target" {
  description = "What custom domains CNAME to (HELP_CNAME_TARGET)."
  value       = cloudflare_dns_record.cname_target.name
}

output "fallback_origin" {
  value = cloudflare_custom_hostname_fallback_origin.this.origin
}
