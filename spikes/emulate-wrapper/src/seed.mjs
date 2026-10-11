export const SIGNING_SECRET = 'localslack-spike-secret';
export const seedConfig = {
  team: { name: 'LocalSlack', domain: 'localslack' },
  users: [
    { name: 'alice', real_name: 'Alice Anderson', email: 'alice@example.com' },
    { name: 'bob', real_name: 'Bob Brown', email: 'bob@example.com' },
    { name: 'carol', real_name: 'Carol Chen', email: 'carol@example.com' },
    { name: 'localslack-bot', real_name: 'LocalSlack Bot', email: 'bot@example.com' }, // flagged is_bot in wrapper
  ],
  channels: [{ name: 'general' }, { name: 'random' }],
  bots: [{ name: 'localslack-bot' }],
  signing_secret: SIGNING_SECRET,
};
export const BOT_NAME = 'localslack-bot';
