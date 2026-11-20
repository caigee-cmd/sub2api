import { describe, expect, it } from 'vitest'
import { normalizeOpenAIEndpointCapabilities } from '@/utils/openaiEndpointCapabilities'

describe('normalizeOpenAIEndpointCapabilities', () => {
  it('keeps messages exclusive when it is newly selected', () => {
    expect(
      normalizeOpenAIEndpointCapabilities(
        ['chat_completions', 'embeddings', 'messages'],
        ['chat_completions', 'embeddings']
      )
    ).toEqual(['messages'])
  })

  it('drops messages when another capability is selected from messages-only', () => {
    expect(
      normalizeOpenAIEndpointCapabilities(['messages', 'embeddings'], ['messages'])
    ).toEqual(['embeddings'])
  })

  it('leaves a messages-only selection unchanged', () => {
    expect(normalizeOpenAIEndpointCapabilities(['messages'], ['messages'])).toEqual(['messages'])
  })

  it('falls back to the default text capabilities when nothing is selected', () => {
    expect(normalizeOpenAIEndpointCapabilities([])).toEqual(['chat_completions', 'embeddings'])
  })
})
