<?php declare(strict_types=1);

return (new PhpCsFixer\Config())
    ->setUsingCache(false)
    ->setRules([
        '@Symfony' => true,

        'blank_line_after_opening_tag' => false,
        'class_attributes_separation' => ['elements' => ['property' => 'one', 'method' => 'one']],
        'concat_space' => ['spacing' => 'one'],
        'fopen_flags' => false,
        'general_phpdoc_annotation_remove' => ['annotations' => ['copyright', 'category']],
        'linebreak_after_opening_tag' => false,
        'method_argument_space' => ['on_multiline' => 'ensure_fully_multiline'],
        'no_superfluous_phpdoc_tags' => ['allow_unused_params' => true, 'allow_mixed' => true],
        'no_useless_else' => true,
        'no_useless_return' => true,
        'ordered_class_elements' => true,
        'phpdoc_align' => ['align' => 'left'],
        'phpdoc_annotation_without_dot' => false,
        'phpdoc_line_span' => true,
        'phpdoc_order' => ['order' => ['param', 'throws', 'return']],
        'phpdoc_summary' => false,
        'phpdoc_to_comment' => false,
        'self_accessor' => false,
        'single_line_throw' => false,
        'single_quote' => ['strings_containing_single_quote_chars' => true],
        'trailing_comma_in_multiline' => ['after_heredoc' => true, 'elements' => ['array_destructuring', 'arrays', 'match']],
        'yoda_style' => [
            'equal' => false,
            'identical' => false,
        ],
    ])
    ->setFinder(PhpCsFixer\Finder::create()
        ->exclude('vendor')
        ->name('/\.php$/')
        ->in(__DIR__),
    )
;
