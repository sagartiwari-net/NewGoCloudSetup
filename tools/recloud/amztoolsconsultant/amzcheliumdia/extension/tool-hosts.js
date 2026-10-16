// One extension serves both Helium plans. The open tool tab picks the host.
var H10_TOOL_HOSTS = [
  "amzcheliumdia.gt4rents.com",
  "amzcheliumplat.gt4rents.com"
];

function h10ToolOrigin(hostname) {
  var host = String(hostname || "").toLowerCase();
  for (var i = 0; i < H10_TOOL_HOSTS.length; i++) {
    if (host === H10_TOOL_HOSTS[i]) return "https://" + H10_TOOL_HOSTS[i];
  }
  return "";
}

globalThis.H10_TOOL_HOSTS = H10_TOOL_HOSTS;
globalThis.h10ToolOrigin = h10ToolOrigin;
