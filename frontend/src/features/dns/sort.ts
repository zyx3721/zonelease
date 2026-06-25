import type { DnsRecord, DnsZone, DnsSortKey, RecordSortState } from './types';

const naturalCollator = new Intl.Collator('zh-CN', {
  numeric: true,
  sensitivity: 'base',
});

export function sortZones(zones: DnsZone[]) {
  return [...zones].sort((left, right) => {
    if (left.reverse !== right.reverse) return left.reverse ? 1 : -1;
    return naturalCompare(left.name, right.name);
  });
}

export function sortRecords(records: DnsRecord[], sort: RecordSortState) {
  const items = [...records];
  if (!sort.direction) return items.sort(defaultRecordCompare);
  const factor = sort.direction === 'asc' ? 1 : -1;
  return items.sort((left, right) => {
    const result =
      sort.key === 'name'
        ? compareRecordName(left.name, right.name)
        : naturalCompare(recordSortValue(left, sort.key), recordSortValue(right, sort.key));
    return result === 0 ? defaultRecordCompare(left, right) : result * factor;
  });
}

function defaultRecordCompare(left: DnsRecord, right: DnsRecord) {
  return compareRecordName(left.name, right.name);
}

function compareRecordName(left: string, right: string) {
  const leftLabels = domainLabels(left);
  const rightLabels = domainLabels(right);
  if (leftLabels.length !== rightLabels.length) return leftLabels.length - rightLabels.length;
  for (let index = 0; index < leftLabels.length; index += 1) {
    const result = naturalCompare(
      leftLabels[leftLabels.length - 1 - index],
      rightLabels[rightLabels.length - 1 - index]
    );
    if (result !== 0) return result;
  }
  return naturalCompare(left, right);
}

function domainLabels(name: string) {
  const clean = name.trim().replace(/\.$/, '');
  if (!clean || clean === '@') return [];
  return clean.split('.').filter(Boolean);
}

function recordSortValue(record: DnsRecord, key: DnsSortKey) {
  if (key === 'type') return record.type;
  if (key === 'value') return record.value;
  return record.name;
}

function naturalCompare(left: string, right: string) {
  return naturalCollator.compare(left.trim(), right.trim());
}
