#!/bin/bash

set -e

SCHEMAS_DIR="schemas"
CHANGELOG_FILES=(tmp/opentelemetry-collector-CHANGELOG*.md)

# Function to extract version from a heading line
extract_version_from_heading() {
    local heading="$1"
    # Remove leading/trailing whitespace and ## prefix
    heading=$(echo "$heading" | sed 's/^[[:space:]]*##[[:space:]]*//' | sed 's/[[:space:]]*$//')

    # Handle dual version format like v1.44.0/v0.138.0 - extract the second (0.x.x) version
    if [[ "$heading" =~ v[0-9]+\.[0-9]+\.[0-9]+/v([0-9]+\.[0-9]+\.[0-9]+) ]]; then
        echo "${BASH_REMATCH[1]}"
    # Handle single version format like v0.138.0, [v0.138.0], 0.138.0, [0.138.0]
    elif [[ "$heading" =~ v?([0-9]+\.[0-9]+\.[0-9]+) ]]; then
        echo "${BASH_REMATCH[1]}"
    fi
}

# Function to extract changelog section for a specific version from a file
extract_version_section() {
    local file="$1"
    local target_version="$2"
    local temp_file=$(mktemp)
    local in_section=false
    local found_version=false

    while IFS= read -r line; do
        # Check if this is a version heading (starts with ##)
        if [[ "$line" =~ ^[[:space:]]*##[[:space:]] ]]; then
            local version=$(extract_version_from_heading "$line")

            if [[ "$version" == "$target_version" ]]; then
                found_version=true
                in_section=true
                echo "$line" >> "$temp_file"
            elif [[ "$in_section" == true ]]; then
                # Hit another ## heading, stop collecting
                break
            fi
        elif [[ "$in_section" == true ]]; then
            # We're in the target section, collect the line
            echo "$line" >> "$temp_file"
        fi
    done < "$file"

    if [[ "$found_version" == true ]]; then
        cat "$temp_file"
    fi

    rm -f "$temp_file"
}

# Function to get versions that exist as directories in schemas/
get_existing_schema_versions() {
    local versions=()

    # Check which version directories already exist in schemas/
    for dir in "$SCHEMAS_DIR"/*/; do
        if [[ -d "$dir" ]]; then
            local version=$(basename "$dir")
            # Check if it looks like a version (contains dots)
            if [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
                versions+=("$version")
            fi
        fi
    done

    printf '%s\n' "${versions[@]}" | sort -V
}

# Function to check if a version exists in changelog files
version_exists_in_changelogs() {
    local target_version="$1"

    for changelog_file in "${CHANGELOG_FILES[@]}"; do
        if [[ -f "$changelog_file" ]]; then
            while IFS= read -r line; do
                if [[ "$line" =~ ^[[:space:]]*##[[:space:]] ]]; then
                    local version=$(extract_version_from_heading "$line")
                    if [[ "$version" == "$target_version" ]]; then
                        return 0  # Found
                    fi
                fi
            done < "$changelog_file"
        fi
    done

    return 1  # Not found
}

# Function to create version-specific changelog file
create_version_changelog() {
    local version="$1"
    local version_dir="$SCHEMAS_DIR/$version"
    local output_file="$version_dir/changelog.md"
    local temp_sections=()

    echo "Processing version $version..."

    # Check if version directory exists (it should, since we filtered for existing ones)
    if [[ ! -d "$version_dir" ]]; then
        echo "  → Error: Version directory $version_dir does not exist"
        return 1
    fi

    # Extract content from all changelog files for this version
    for changelog_file in "${CHANGELOG_FILES[@]}"; do
        if [[ -f "$changelog_file" ]]; then
            local section_content=$(extract_version_section "$changelog_file" "$version")
            if [[ -n "$section_content" ]]; then
                local temp_section=$(mktemp)
                echo "$section_content" > "$temp_section"
                temp_sections+=("$temp_section")
            fi
        fi
    done

    # Combine all sections into the output file
    if [[ ${#temp_sections[@]} -gt 0 ]]; then
        > "$output_file"  # Clear the output file

        for i in "${!temp_sections[@]}"; do
            cat "${temp_sections[$i]}" >> "$output_file"
            # Add separator between sections (but not after the last one)
            if [[ $i -lt $((${#temp_sections[@]} - 1)) ]]; then
                echo "" >> "$output_file"
                echo "" >> "$output_file"
            fi
        done

        echo "  → Created $output_file"

        # Clean up temp files
        for temp_file in "${temp_sections[@]}"; do
            rm -f "$temp_file"
        done
    else
        echo "  → No content found for version $version"
    fi
}

# Main execution
main() {
    echo "Scanning changelog files: ${CHANGELOG_FILES[*]}"

    # Check if changelog files exist
    local files_exist=false
    for changelog_file in "${CHANGELOG_FILES[@]}"; do
        if [[ -f "$changelog_file" ]]; then
            files_exist=true
            echo "Found changelog file: $changelog_file"
        fi
    done

    if [[ "$files_exist" != true ]]; then
        echo "No changelog files found in $SCHEMAS_DIR directory"
        exit 1
    fi

    # Get versions that exist as directories in schemas/
    local existing_versions=($(get_existing_schema_versions))

    if [[ ${#existing_versions[@]} -eq 0 ]]; then
        echo "No existing version directories found in $SCHEMAS_DIR"
        exit 1
    fi

    echo "Found ${#existing_versions[@]} existing version directories: ${existing_versions[*]}"
    echo ""

    # Process each existing version, but only if it has changelog content
    local processed_count=0
    for version in "${existing_versions[@]}"; do
        if version_exists_in_changelogs "$version"; then
            create_version_changelog "$version"
            processed_count=$((processed_count + 1))
        else
            echo "  → Skipping version $version (no changelog content found)"
        fi
    done

    echo ""
    if [[ $processed_count -eq 0 ]]; then
        echo "No changelog content found for any existing version directories"
    else
        echo "Processed $processed_count versions with changelog content"
    fi

    echo ""
    echo "Changelog parsing completed successfully!"
}

# Run main function
main "$@"