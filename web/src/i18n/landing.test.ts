import { describe, it, expect } from 'vitest'
import { en } from './en'
import { ru } from './ru'

describe('i18n translations', () => {
  it('en and ru have the same keys', () => {
    const enKeys = Object.keys(en).sort()
    const ruKeys = Object.keys(ru).sort()
    expect(enKeys).toEqual(ruKeys)
  })

  it('no empty values in en', () => {
    Object.entries(en).forEach(([key, val]) => {
      expect(val.length, `en.${key} is empty`).toBeGreaterThan(0)
    })
  })

  it('no empty values in ru', () => {
    Object.entries(ru).forEach(([key, val]) => {
      expect(val.length, `ru.${key} is empty`).toBeGreaterThan(0)
    })
  })
})
