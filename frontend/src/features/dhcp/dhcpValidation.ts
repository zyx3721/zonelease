export function validateScopeRange(
  subnet: string,
  startRange: string,
  endRange: string,
  currentStartRange?: string,
  currentEndRange?: string
) {
  const start = parseIPv4(startRange);
  const end = parseIPv4(endRange);
  if (start === null || end === null) return '地址范围必须是有效的 IPv4 地址';
  if (start > end) return '起始 IP 地址不能大于结束 IP 地址';

  const parsedSubnet = parseSubnet(subnet);
  if (!parsedSubnet) return '';
  const { network, broadcast, mask } = parsedSubnet;
  if ((start & mask) !== network || (end & mask) !== network) {
    return '地址范围必须在当前作用域子网内';
  }
  if (start <= network || end >= broadcast) {
    return '地址范围不能包含子网地址或广播地址';
  }

  const currentStart = currentStartRange ? parseIPv4(currentStartRange) : null;
  const currentEnd = currentEndRange ? parseIPv4(currentEndRange) : null;
  if (currentStart !== null && currentEnd !== null) {
    const startChanged = start !== currentStart;
    const endChanged = end !== currentEnd;
    if (startChanged && endChanged) {
      const bothShrink = start > currentStart && end < currentEnd;
      const bothExpand = start < currentStart && end > currentEnd;
      if (!bothShrink && !bothExpand) {
        return '同时修改起始和结束 IP 时，只能同时缩小或同时扩大范围';
      }
    }
    if (!startChanged && endChanged) {
      return '';
    }
    if (startChanged && !endChanged) {
      return '';
    }
  }
  return '';
}

export function validateSubnetRange(subnet: string, startRange: string, endRange: string) {
  const start = parseIPv4(startRange);
  const end = parseIPv4(endRange);
  if (start === null || end === null) return '地址范围必须是有效的 IPv4 地址';
  if (start > end) return '起始 IP 地址不能大于结束 IP 地址';
  const parsedSubnet = parseSubnet(subnet);
  if (!parsedSubnet) return '子网必须是有效的 IPv4 CIDR 或子网掩码格式';
  const { network, broadcast, mask } = parsedSubnet;
  if ((start & mask) !== network || (end & mask) !== network) {
    return '地址范围必须在当前作用域子网内';
  }
  if (start <= network || end >= broadcast) {
    return '地址范围不能包含子网地址或广播地址';
  }
  return '';
}

export function validateDefaultGateway(subnet: string, defaultGateway: string) {
  const gateway = defaultGateway.trim();
  if (!gateway) return '默认网关不能为空';
  const gatewayIP = parseIPv4(gateway);
  if (gatewayIP === null) return '默认网关必须是有效的 IPv4 地址';
  const parsedSubnet = parseSubnet(subnet);
  if (!parsedSubnet) return '子网必须是有效的 IPv4 CIDR 或子网掩码格式';
  const { network, broadcast, mask } = parsedSubnet;
  if ((gatewayIP & mask) !== network) return '默认网关必须在当前作用域子网内';
  if (gatewayIP <= network || gatewayIP >= broadcast) return '默认网关不能是子网地址或广播地址';
  return '';
}

export function validateScopeSubnetConflict(
  subnet: string,
  existingScopes: Array<{ subnet: string }>
) {
  const nextSubnet = parseSubnet(subnet);
  if (!nextSubnet) return '子网必须是有效的 IPv4 CIDR 或子网掩码格式';
  const nextStart = nextSubnet.network;
  const nextEnd = nextSubnet.broadcast;
  const hasConflict = existingScopes.some(scope => {
    const existing = parseSubnet(scope.subnet);
    if (!existing) return false;
    return nextStart <= existing.broadcast && nextEnd >= existing.network;
  });
  return hasConflict ? '作用域子网不能与已有作用域重复或重叠' : '';
}

export function validateExclusionRange({
  startIp,
  endIp,
  scopeStartRange,
  scopeEndRange,
  existingExclusions,
}: {
  startIp: string;
  endIp: string;
  scopeStartRange: string;
  scopeEndRange: string;
  existingExclusions: Array<{ startIp: string; endIp: string }>;
}) {
  const start = parseIPv4(startIp);
  const end = parseIPv4(endIp);
  if (start === null || end === null) return '排除范围必须是有效的 IPv4 地址';
  if (start > end) return '起始 IP 地址不能大于结束 IP 地址';

  const scopeStart = parseIPv4(scopeStartRange);
  const scopeEnd = parseIPv4(scopeEndRange);
  if (scopeStart !== null && scopeEnd !== null && (start < scopeStart || end > scopeEnd)) {
    return '排除范围必须在当前作用域地址范围内';
  }

  const overlap = existingExclusions.some(exclusion => {
    const exclusionStart = parseIPv4(exclusion.startIp);
    const exclusionEnd = parseIPv4(exclusion.endIp);
    if (exclusionStart === null || exclusionEnd === null) return false;
    return start <= exclusionEnd && end >= exclusionStart;
  });
  if (overlap) return '排除范围不能与已有排除范围重叠';
  return '';
}

function parseSubnet(subnet: string) {
  const [networkText, maskText] = subnet.split('/').map(part => part.trim());
  const networkIP = parseIPv4(networkText);
  if (networkIP === null || !maskText) return null;
  let mask: number | null = null;
  if (/^\d+$/.test(maskText)) {
    const prefix = Number(maskText);
    if (prefix < 0 || prefix > 32) return null;
    mask = prefix === 0 ? 0 : (0xffffffff << (32 - prefix)) >>> 0;
  } else {
    mask = parseIPv4(maskText);
  }
  if (mask === null) return null;
  const network = (networkIP & mask) >>> 0;
  return { network, broadcast: (network | (~mask >>> 0)) >>> 0, mask };
}

export function parseIPv4(value: string) {
  const parts = value.trim().split('.');
  if (parts.length !== 4) return null;
  let result = 0;
  for (const part of parts) {
    if (!/^\d+$/.test(part)) return null;
    const number = Number(part);
    if (number < 0 || number > 255) return null;
    result = ((result << 8) | number) >>> 0;
  }
  return result;
}
