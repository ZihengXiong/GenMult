import { describe, expect, it } from 'vitest'
import { tomlStringLiteral } from './local-server'

describe('tomlStringLiteral', () => {
  it('escapes Windows paths for TOML basic strings', () => {
    expect(tomlStringLiteral('C:\\Users\\Dustella\\AppData\\Roaming\\Memoh\\data\\local'))
      .toBe('"C:\\\\Users\\\\Dustella\\\\AppData\\\\Roaming\\\\Memoh\\\\data\\\\local"')
  })

  it('escapes quotes and control characters', () => {
    expect(tomlStringLiteral('quoted "value"\nnext'))
      .toBe('"quoted \\"value\\"\\nnext"')
  })
})
