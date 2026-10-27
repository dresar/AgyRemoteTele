const ANSI_REGEX = /\x1B(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~])/g;
const XML_TAGS_REGEX = /<\/?(USER_REQUEST|ADDITIONAL_METADATA|USER_SETTINGS_CHANGE|SYSTEM_MESSAGE)[^>]*>/gi;
const XML_BLOCKS_REGEX = /<(USER_REQUEST|ADDITIONAL_METADATA|USER_SETTINGS_CHANGE|SYSTEM_MESSAGE)[\s\S]*?<\/\1>/gi;

export function stripClutter(text: string): string {
  if (!text) return '';
  let cleaned = text.replace(ANSI_REGEX, '');
  cleaned = cleaned.replace(XML_BLOCKS_REGEX, '');
  cleaned = cleaned.replace(XML_TAGS_REGEX, '');
  return cleaned;
}

export function cleanPrompt(raw: string): string {
  if (!raw) return '';
  const match = raw.match(/<USER_REQUEST>([\s\S]*?)<\/USER_REQUEST>/i);
  let content = match ? match[1] : raw;
  content = stripClutter(content).trim();
  const firstLine = content.split('\n')[0]?.trim() || '';
  if (firstLine.length > 120) {
    return firstLine.slice(0, 117) + '...';
  }
  return firstLine || content;
}

export function executiveSummary(raw: string, maxLen = 300): string {
  if (!raw) return '';
  const cleaned = stripClutter(raw).trim();
  const paragraphs = cleaned.split(/\n\s*\n/).map(p => p.trim()).filter(Boolean);
  const target = paragraphs[0] || cleaned;
  if (target.length <= maxLen) {
    return target;
  }
  return target.slice(0, maxLen - 3) + '...';
}
