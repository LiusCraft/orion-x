-- 补全缺失的方言标签（挂载到 zh 下）
INSERT INTO languages (code, name, parent_code, created_at, updated_at, creator)
VALUES
    ('yue',        'Cantonese (粤语)',           'zh', now(), now(), 'system'),
    ('zh-minnan',  'Southern Min (闽南语)',      'zh', now(), now(), 'system'),
    ('zh-dongbei', 'Northeastern Chinese (东北话)', 'zh', now(), now(), 'system'),
    ('zh-henan',   'Henanese (河南话)',          'zh', now(), now(), 'system'),
    ('zh-hunan',   'Xiang Chinese (湖南话)',     'zh', now(), now(), 'system'),
    ('zh-shaanxi', 'Shaanxi Chinese (陕西话)',   'zh', now(), now(), 'system'),
    ('zh-shandong','Shandong Chinese (山东话)',  'zh', now(), now(), 'system'),
    ('zh-sichuan', 'Sichuan Chinese (四川话)',   'zh', now(), now(), 'system'),
    ('zh-anhui',   'Anhui Chinese (安徽话)',     'zh', now(), now(), 'system')
ON CONFLICT (code) DO NOTHING;

-- 同时加到 seed 列表，下次迁移时自动补上
