const directPhrases = [
  '加我微信',
  '加下微信',
  '微信联系',
  '留个微信',
  '交换微信',
  'vx联系',
  'v信联系',
  '加我qq',
  'qq联系',
  '留个qq',
  '私下联系',
  '线下联系',
  '站外联系',
  '加我好友',
  '扫码加我',
  '扫二维码',
  '联系方式发我',
  '留个联系方式',
  '代开发票',
  '买卖银行卡',
  '收购银行卡',
  '兼职跑分',
  '刷单返利',
  '代办假证',
  '博彩代理',
  '网赌代理',
];

const contextualTerms = [
  '傻逼', '煞笔', '沙比', '蠢货', '废物', '垃圾人', '脑残', '弱智', '智障',
  '贱人', '婊子', '去死', '弄死你', '杀了你', '打死你', '人肉你', '曝光你',
  '跟踪你', '操你', '草你', '妈的', '滚蛋', '滚开', '骗钱', '转账', '汇款',
  '保证金', '返利', '下注', '博彩', '毒品', '迷药',
];

const url = /https?:\/\/|www\.|(?:[a-z0-9-]+\.)+(?:com|cn|net|org|io|me)(?:[/\s]|$)/i;
const email = /[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}/i;
const phone = /(?:^|\D)(?:\+?86[- ]?)?1[3-9]\d{9}(?:\D|$)/;
const qq = /(?:qq|q q|扣扣|企鹅)\s*(?:号|号码|：|:|是|加)?\s*[1-9]\d{4,11}/i;

export function checkMessage(text) {
  const normalized = text.toLowerCase().replace(/\s+/g, '');
  if (
    url.test(text) ||
    email.test(text) ||
    phone.test(text) ||
    qq.test(text) ||
    directPhrases.some((phrase) => normalized.includes(phrase))
  ) {
    return { action: 'reject', message: '消息包含联系方式、站外引导或明确违规内容，请修改后再发送。' };
  }
  if (contextualTerms.some((term) => normalized.includes(term))) {
    return { action: 'needs_ai', message: '这条消息需要 AI 判断语境，当前演示环境未配置 AI，没有发送。' };
  }
  return { action: 'allow', message: '' };
}
