import type { OpenAIEndpointCapability } from '@/types'

export const OPENAI_ENDPOINT_CAPABILITIES: OpenAIEndpointCapability[] = [
  'chat_completions',
  'embeddings',
  'messages',
  'seedance',
  'videos'
]

const DEFAULT_OPENAI_ENDPOINT_CAPABILITIES: OpenAIEndpointCapability[] = [
  'chat_completions',
  'embeddings'
]

// Messages（仅 /v1/messages）与其他端点互斥。
// 勾上 messages 时只留它；已经是仅 messages 时再勾其他，则去掉 messages。
export const normalizeOpenAIEndpointCapabilities = (
  values: OpenAIEndpointCapability[],
  previous: OpenAIEndpointCapability[] = []
): OpenAIEndpointCapability[] => {
  const selected = OPENAI_ENDPOINT_CAPABILITIES.filter((value) => values.includes(value))
  if (selected.includes('messages') && selected.length > 1) {
    if (previous.length === 1 && previous[0] === 'messages') {
      return selected.filter((value) => value !== 'messages')
    }
    return ['messages']
  }
  return selected.length > 0 ? selected : [...DEFAULT_OPENAI_ENDPOINT_CAPABILITIES]
}
