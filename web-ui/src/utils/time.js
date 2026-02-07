export function formatTime(ts) {
  const date = new Date(ts);
  const hh = String(date.getHours()).padStart(2, '0');
  const mm = String(date.getMinutes()).padStart(2, '0');
  return hh + ':' + mm;
}

export function formatRelative(ts) {
  const diff = Date.now() - ts;
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return '刚刚';
  if (minutes < 60) return String(minutes) + '分钟前';
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return String(hours) + '小时前';
  const days = Math.floor(hours / 24);
  return String(days) + '天前';
}
